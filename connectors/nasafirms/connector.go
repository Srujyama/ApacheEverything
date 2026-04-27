// Package nasafirms is a Sunny connector for NASA's Fire Information for
// Resource Management System (FIRMS). It fetches near-real-time active-fire
// detections from VIIRS and MODIS satellites.
//
// FIRMS requires a free API key (MAP_KEY) from
// https://firms.modaps.eosdis.nasa.gov/api/. Set it as
//
//	SUNNY_SECRET_NASA_FIRMS_KEY
//
// If the secret is missing, the connector logs once and idles — the runtime
// keeps the instance alive so the user can fix the secret without a restart.
package nasafirms

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
	"github.com/sunny/sunny/packages/sdk-go/sdkhttp"
)

const (
	ID      = "nasa-firms"
	Version = "0.1.0"

	// SecretKey is the secret name the connector reads.
	SecretKey = "nasa-firms-key"
)

type Config struct {
	// Source: VIIRS_SNPP_NRT, VIIRS_NOAA20_NRT, MODIS_NRT, ...
	Source string `json:"source"`

	// Area: country code (e.g. "USA", "BRA") or "world".
	Area string `json:"area"`

	// DayRange: how many past days to fetch each poll. 1..10. Default 1.
	DayRange int `json:"dayRange"`

	// PollMinutes: how often to poll. Default 30 (FIRMS NRT updates ~every
	// few hours; polling more often just wastes the daily quota).
	PollMinutes int `json:"pollMinutes"`
}

func (c *Config) applyDefaults() {
	if c.Source == "" {
		c.Source = "VIIRS_SNPP_NRT"
	}
	if c.Area == "" {
		c.Area = "USA"
	}
	if c.DayRange <= 0 {
		c.DayRange = 1
	}
	if c.PollMinutes <= 0 {
		c.PollMinutes = 30
	}
}

func (c *Config) feedURL(key string) string {
	return fmt.Sprintf(
		"https://firms.modaps.eosdis.nasa.gov/api/area/csv/%s/%s/%s/%d",
		key, c.Source, c.Area, c.DayRange,
	)
}

type Connector struct{ http *sdkhttp.Client }

func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }

func (c *Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "NASA FIRMS Active Fires",
		Version:     Version,
		Category:    sdk.CategoryWildfire,
		Mode:        sdk.ModePull,
		Description: "Active-fire detections from NASA FIRMS (VIIRS/MODIS). Requires a free MAP_KEY.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"source": {"type": "string", "default": "VIIRS_SNPP_NRT"},
				"area":   {"type": "string", "default": "USA"},
				"dayRange": {"type": "integer", "minimum": 1, "maximum": 10, "default": 1},
				"pollMinutes": {"type": "integer", "minimum": 5, "default": 30}
			}
		}`),
	}
}

func (c *Connector) Validate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("nasa-firms config: %w", err)
	}
	if cfg.DayRange < 0 || cfg.DayRange > 10 {
		return errors.New("dayRange must be between 1 and 10")
	}
	return nil
}

const checkpointKey = "lastEmittedKey"

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()

	key := rt.Secret(SecretKey)
	if key == "" {
		// Idle, but don't crash. Log once and check periodically — phase 4's
		// UI will let users add a secret without restarting.
		rt.Logger().Warn("nasa-firms: no MAP_KEY secret; idling. Set SUNNY_SECRET_NASA_FIRMS_KEY to enable.")
		return idleUntilCancel(ctx)
	}

	rt.Logger().Info("nasa-firms starting", "source", cfg.Source, "area", cfg.Area, "pollMinutes", cfg.PollMinutes)

	lastKey, _ := rt.LoadCheckpoint(ctx, checkpointKey)

	tick := time.NewTicker(time.Duration(cfg.PollMinutes) * time.Minute)
	defer tick.Stop()

	lastKey = c.pollOnce(ctx, rt, cfg, key, lastKey)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			lastKey = c.pollOnce(ctx, rt, cfg, key, lastKey)
		}
	}
}

func idleUntilCancel(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (c *Connector) pollOnce(ctx context.Context, rt sdk.Context, cfg Config, apiKey, lastKey string) string {
	body, err := c.http.Get(ctx, cfg.feedURL(apiKey), map[string]string{"Accept": "text/csv"})
	if err != nil {
		rt.Logger().Warn("poll", "err", err)
		return lastKey
	}

	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1 // FIRMS sometimes adds optional columns
	header, err := r.Read()
	if err != nil {
		rt.Logger().Warn("read csv header", "err", err)
		return lastKey
	}
	col := indexHeader(header)

	emitted := 0
	maxKey := lastKey
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rt.Logger().Warn("read csv row", "err", err)
			break
		}

		// Compose a stable per-detection key: time + lat + lng + sat. FIRMS
		// rows don't have a unique ID. This dedups across overlapping polls.
		date := getCol(row, col, "acq_date")
		atime := getCol(row, col, "acq_time")
		latStr := getCol(row, col, "latitude")
		lngStr := getCol(row, col, "longitude")
		sat := getCol(row, col, "satellite")
		recKey := fmt.Sprintf("%s|%s|%s|%s|%s", date, atime, latStr, lngStr, sat)

		if recKey <= lastKey {
			continue
		}

		lat, _ := strconv.ParseFloat(latStr, 64)
		lng, _ := strconv.ParseFloat(lngStr, 64)
		bright, _ := strconv.ParseFloat(getCol(row, col, "bright_ti4"), 64)
		if bright == 0 {
			bright, _ = strconv.ParseFloat(getCol(row, col, "brightness"), 64)
		}
		frp, _ := strconv.ParseFloat(getCol(row, col, "frp"), 64) // fire radiative power MW
		conf := getCol(row, col, "confidence")

		ts := parseFIRMSTime(date, atime)

		tags := map[string]string{
			"source":     cfg.Source,
			"satellite":  sat,
			"confidence": conf,
			"daynight":   getCol(row, col, "daynight"),
		}
		payload, _ := json.Marshal(map[string]any{
			"date":       date,
			"time":       atime,
			"brightness": bright,
			"frp":        frp,
			"track":      getCol(row, col, "track"),
			"scan":       getCol(row, col, "scan"),
		})

		err = rt.Publish(ctx, sdk.Record{
			Timestamp: ts,
			SourceID:  recKey,
			Location:  &sdk.GeoPoint{Lat: lat, Lng: lng},
			Tags:      tags,
			Payload:   payload,
		})
		if err != nil {
			rt.Logger().Warn("publish", "err", err)
			return maxKey
		}
		emitted++
		if recKey > maxKey {
			maxKey = recKey
		}
	}

	if emitted > 0 {
		rt.Logger().Info("nasa-firms emitted", "count", emitted)
		_ = rt.Checkpoint(ctx, checkpointKey, maxKey)
	}
	return maxKey
}

func indexHeader(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func getCol(row []string, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// parseFIRMSTime parses date "YYYY-MM-DD" and time "HHMM" to a UTC time.Time.
func parseFIRMSTime(date, atime string) time.Time {
	if len(atime) == 3 {
		atime = "0" + atime
	}
	if len(atime) != 4 {
		return time.Now().UTC()
	}
	t, err := time.Parse("2006-01-02 1504", date+" "+atime)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}
