// Package openaq is a Sunny connector for the OpenAQ v3 API.
//
// OpenAQ v3 requires a free API key from https://api.openaq.org/. Set it as
//
//	SUNNY_SECRET_OPENAQ_API_KEY
//
// The connector fetches recent measurements for a configured bbox/country.
// Like nasa-firms, it idles gracefully if the secret isn't set.
package openaq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
	"github.com/sunny/sunny/packages/sdk-go/sdkhttp"
)

const (
	ID      = "openaq"
	Version = "0.1.0"

	// SecretKey is the secret name the connector reads.
	SecretKey = "openaq-api-key"
)

type Config struct {
	// CountriesID filters to one country by OpenAQ country ID. 155 = USA.
	// Either set this or BBox.
	CountriesID int `json:"countriesId"`

	// BBox is "minLng,minLat,maxLng,maxLat". Overrides CountriesID if set.
	BBox string `json:"bbox"`

	// LimitPerPoll caps how many measurements to fetch per poll. Default 200.
	LimitPerPoll int `json:"limitPerPoll"`

	// PollMinutes is how often to poll. Default 15. OpenAQ updates roughly
	// hourly so polling more frequently just burns API quota.
	PollMinutes int `json:"pollMinutes"`
}

func (c *Config) applyDefaults() {
	if c.CountriesID == 0 && c.BBox == "" {
		c.CountriesID = 155 // USA
	}
	if c.LimitPerPoll <= 0 {
		c.LimitPerPoll = 200
	}
	if c.PollMinutes <= 0 {
		c.PollMinutes = 15
	}
}

func (c *Config) feedURL() string {
	v := url.Values{}
	v.Set("limit", strconv.Itoa(c.LimitPerPoll))
	v.Set("order_by", "datetime")
	v.Set("sort_order", "desc")
	if c.BBox != "" {
		v.Set("bbox", c.BBox)
	} else if c.CountriesID > 0 {
		v.Set("countries_id", strconv.Itoa(c.CountriesID))
	}
	return "https://api.openaq.org/v3/measurements?" + v.Encode()
}

type Connector struct{ http *sdkhttp.Client }

func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }

func (c *Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "OpenAQ Air Quality",
		Version:     Version,
		Category:    sdk.CategoryAirQuality,
		Mode:        sdk.ModePull,
		Description: "Air-quality measurements from OpenAQ v3. Requires a free API key.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"countriesId": {"type": "integer", "default": 155, "description": "OpenAQ country id; 155 = USA"},
				"bbox": {"type": "string", "description": "minLng,minLat,maxLng,maxLat — overrides countriesId"},
				"limitPerPoll": {"type": "integer", "default": 200},
				"pollMinutes": {"type": "integer", "minimum": 1, "default": 15}
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
		return fmt.Errorf("openaq config: %w", err)
	}
	if cfg.PollMinutes < 0 {
		return errors.New("pollMinutes must be >= 0")
	}
	return nil
}

type measurement struct {
	ID         int64  `json:"id"`
	LocationID int64  `json:"locationsId"`
	SensorsID  int64  `json:"sensorsId"`
	Value      float64 `json:"value"`
	Parameter  struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Units string `json:"units"`
	} `json:"parameter"`
	Period struct {
		DatetimeFrom struct {
			UTC string `json:"utc"`
		} `json:"datetimeFrom"`
		DatetimeTo struct {
			UTC string `json:"utc"`
		} `json:"datetimeTo"`
	} `json:"period"`
	Coordinates struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"coordinates"`
}

type measurementsResp struct {
	Results []measurement `json:"results"`
}

const checkpointKey = "lastSeenISO"

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()

	apiKey := rt.Secret(SecretKey)
	if apiKey == "" {
		rt.Logger().Warn("openaq: no API key; idling. Set SUNNY_SECRET_OPENAQ_API_KEY to enable.")
		<-ctx.Done()
		return ctx.Err()
	}

	rt.Logger().Info("openaq starting", "countriesId", cfg.CountriesID, "pollMinutes", cfg.PollMinutes)
	lastSeen, _ := rt.LoadCheckpoint(ctx, checkpointKey)

	tick := time.NewTicker(time.Duration(cfg.PollMinutes) * time.Minute)
	defer tick.Stop()

	lastSeen = c.pollOnce(ctx, rt, cfg, apiKey, lastSeen)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			lastSeen = c.pollOnce(ctx, rt, cfg, apiKey, lastSeen)
		}
	}
}

func (c *Connector) pollOnce(ctx context.Context, rt sdk.Context, cfg Config, apiKey, lastSeen string) string {
	body, err := c.http.GetJSON(ctx, cfg.feedURL(), map[string]string{"X-API-Key": apiKey})
	if err != nil {
		rt.Logger().Warn("poll", "err", err)
		return lastSeen
	}
	var resp measurementsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		rt.Logger().Warn("decode", "err", err)
		return lastSeen
	}

	emitted := 0
	maxSeen := lastSeen
	for _, m := range resp.Results {
		dtStr := m.Period.DatetimeTo.UTC
		if dtStr == "" {
			dtStr = m.Period.DatetimeFrom.UTC
		}
		if dtStr <= lastSeen {
			continue
		}
		ts, err := time.Parse(time.RFC3339, dtStr)
		if err != nil {
			continue
		}

		var loc *sdk.GeoPoint
		if m.Coordinates.Latitude != nil && m.Coordinates.Longitude != nil {
			loc = &sdk.GeoPoint{Lat: *m.Coordinates.Latitude, Lng: *m.Coordinates.Longitude}
		}

		tags := map[string]string{
			"parameter": m.Parameter.Name,
			"unit":      m.Parameter.Units,
			"locationId": strconv.FormatInt(m.LocationID, 10),
			"sensorId":   strconv.FormatInt(m.SensorsID, 10),
		}
		payload, _ := json.Marshal(map[string]any{
			"value":     m.Value,
			"parameter": m.Parameter.Name,
			"unit":      m.Parameter.Units,
		})

		err = rt.Publish(ctx, sdk.Record{
			Timestamp: ts.UTC(),
			SourceID:  strconv.FormatInt(m.SensorsID, 10),
			Location:  loc,
			Tags:      tags,
			Payload:   payload,
		})
		if err != nil {
			rt.Logger().Warn("publish", "err", err)
			return maxSeen
		}
		emitted++
		if dtStr > maxSeen {
			maxSeen = dtStr
		}
	}

	if emitted > 0 {
		rt.Logger().Info("openaq emitted", "count", emitted)
		_ = rt.Checkpoint(ctx, checkpointKey, maxSeen)
	}
	return maxSeen
}
