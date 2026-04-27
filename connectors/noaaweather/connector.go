// Package noaaweather is a Sunny connector for NOAA's National Weather
// Service active-alerts API. No auth required.
//
// Each active alert becomes one record per poll. We dedup by alert ID +
// `sent` timestamp checkpoint so updates to an existing alert reach the
// pipeline (NWS revises alerts in place).
package noaaweather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
	"github.com/sunny/sunny/packages/sdk-go/sdkhttp"
)

const (
	ID      = "noaa-weather-alerts"
	Version = "0.1.0"

	feedURL = "https://api.weather.gov/alerts/active"
)

type Config struct {
	// PollSeconds is how often to fetch /alerts/active. Default 60.
	PollSeconds int `json:"pollSeconds"`

	// SeverityMin is the lowest severity to emit. One of:
	// Unknown, Minor, Moderate, Severe, Extreme. Default "" (all).
	SeverityMin string `json:"severityMin"`

	// SkipTestMessages drops alerts with status="Test" (NWS keepalives).
	// Default true.
	SkipTestMessages *bool `json:"skipTestMessages"`
}

func (c *Config) applyDefaults() {
	if c.PollSeconds <= 0 {
		c.PollSeconds = 60
	}
	if c.SkipTestMessages == nil {
		t := true
		c.SkipTestMessages = &t
	}
}

type Connector struct{ http *sdkhttp.Client }

func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }

func (c *Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "NOAA Weather Alerts",
		Version:     Version,
		Category:    sdk.CategoryWeather,
		Mode:        sdk.ModePull,
		Description: "Pulls active alerts from the NWS api.weather.gov.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pollSeconds": {"type": "integer", "minimum": 30, "default": 60},
				"severityMin": {"type": "string", "enum": ["", "Unknown", "Minor", "Moderate", "Severe", "Extreme"], "default": ""},
				"skipTestMessages": {"type": "boolean", "default": true}
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
		return fmt.Errorf("noaa-weather-alerts config: %w", err)
	}
	if cfg.PollSeconds < 0 {
		return errors.New("pollSeconds must be >= 0")
	}
	return nil
}

// Severities ordered low → high. Empty string means "any".
var severityOrder = map[string]int{
	"":         -1,
	"Unknown":  0,
	"Minor":    1,
	"Moderate": 2,
	"Severe":   3,
	"Extreme":  4,
}

type alertFeature struct {
	ID         string `json:"id"`
	Properties struct {
		Event       string `json:"event"`
		Severity    string `json:"severity"`
		Certainty   string `json:"certainty"`
		Urgency     string `json:"urgency"`
		Headline    string `json:"headline"`
		Description string `json:"description"`
		Sender      string `json:"senderName"`
		AreaDesc    string `json:"areaDesc"`
		Status      string `json:"status"`
		Sent        string `json:"sent"`      // RFC3339
		Effective   string `json:"effective"` // RFC3339
		Expires     string `json:"expires"`   // RFC3339
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"` // polygon/multipolygon, may be null
}

type alertCollection struct {
	Features []alertFeature `json:"features"`
}

const checkpointKey = "lastSeen"

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()
	rt.Logger().Info("noaa-weather-alerts starting", "pollSeconds", cfg.PollSeconds, "severityMin", cfg.SeverityMin)

	seen, err := loadSeen(ctx, rt)
	if err != nil {
		rt.Logger().Warn("load checkpoint", "err", err)
	}

	tick := time.NewTicker(time.Duration(cfg.PollSeconds) * time.Second)
	defer tick.Stop()

	c.pollOnce(ctx, rt, cfg, seen)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			c.pollOnce(ctx, rt, cfg, seen)
		}
	}
}

// pollOnce fetches the feed and emits unseen alerts. seen is a map of
// alertID -> last "sent" string we emitted; updates with newer sent times
// re-emit so users get amended alerts.
func (c *Connector) pollOnce(ctx context.Context, rt sdk.Context, cfg Config, seen map[string]string) {
	body, err := c.http.GetJSON(ctx, feedURL, nil)
	if err != nil {
		rt.Logger().Warn("poll", "err", err)
		return
	}
	var fc alertCollection
	if err := json.Unmarshal(body, &fc); err != nil {
		rt.Logger().Warn("decode", "err", err)
		return
	}

	emitted := 0
	skipTest := cfg.SkipTestMessages != nil && *cfg.SkipTestMessages
	min := severityOrder[cfg.SeverityMin]

	for _, a := range fc.Features {
		if skipTest && a.Properties.Status == "Test" {
			continue
		}
		if order, ok := severityOrder[a.Properties.Severity]; ok && order < min {
			continue
		}
		if seen[a.ID] == a.Properties.Sent && a.Properties.Sent != "" {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, a.Properties.Sent)
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		tags := map[string]string{
			"event":     a.Properties.Event,
			"severity":  strings.ToLower(a.Properties.Severity),
			"certainty": strings.ToLower(a.Properties.Certainty),
			"urgency":   strings.ToLower(a.Properties.Urgency),
			"status":    strings.ToLower(a.Properties.Status),
		}

		payload, _ := json.Marshal(map[string]any{
			"alertId":     a.ID,
			"event":       a.Properties.Event,
			"headline":    a.Properties.Headline,
			"description": a.Properties.Description,
			"areaDesc":    a.Properties.AreaDesc,
			"sender":      a.Properties.Sender,
			"sent":        a.Properties.Sent,
			"effective":   a.Properties.Effective,
			"expires":     a.Properties.Expires,
		})

		// We don't compute a centroid for polygons here — that's for the
		// LiveMap layer in phase 5. For now leave Location nil for area
		// alerts; the areaDesc field carries the human-readable region.
		err := rt.Publish(ctx, sdk.Record{
			Timestamp: ts.UTC(),
			SourceID:  a.ID,
			Tags:      tags,
			Payload:   payload,
		})
		if err != nil {
			rt.Logger().Warn("publish", "err", err)
			return
		}
		seen[a.ID] = a.Properties.Sent
		emitted++
	}

	if emitted > 0 {
		rt.Logger().Info("noaa-weather-alerts emitted", "count", emitted)
		_ = saveSeen(ctx, rt, seen)
	}
}

func loadSeen(ctx context.Context, rt sdk.Context) (map[string]string, error) {
	v, err := rt.LoadCheckpoint(ctx, checkpointKey)
	if err != nil || v == "" {
		return map[string]string{}, err
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(v), &m); err != nil {
		return map[string]string{}, err
	}
	return m, nil
}

func saveSeen(ctx context.Context, rt sdk.Context, seen map[string]string) error {
	// Cap at 5000 entries to keep the checkpoint small. We trim the oldest
	// by dropping a random subset; alerts that fall out get re-emitted, which
	// is fine.
	if len(seen) > 5000 {
		i := 0
		for k := range seen {
			delete(seen, k)
			i++
			if i >= len(seen)-4500 {
				break
			}
		}
	}
	b, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	return rt.Checkpoint(ctx, checkpointKey, string(b))
}
