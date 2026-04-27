// Package usgsearthquakes is a Sunny connector for the USGS Earthquake
// Hazards Program GeoJSON feeds. No auth needed.
//
// Default feed: all earthquakes in the past hour, polled every 60 seconds.
// Other feeds (significant_month, all_day, etc.) can be selected via config.
package usgsearthquakes

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
	ID      = "usgs-earthquakes"
	Version = "0.1.0"
)

// Config is what users put under `config:` in sunny.config.yaml.
type Config struct {
	// Feed is the USGS feed name. One of:
	//   all_hour, all_day, all_week, all_month,
	//   significant_hour, significant_day, significant_week, significant_month,
	//   M1.0_hour, M2.5_day, M4.5_week, ...
	Feed string `json:"feed"`

	// PollSeconds is how often to fetch the feed. Default 60.
	PollSeconds int `json:"pollSeconds"`

	// MinMagnitude filters out events below this magnitude. Default 0 (all).
	MinMagnitude float64 `json:"minMagnitude"`
}

func (c *Config) applyDefaults() {
	if c.Feed == "" {
		c.Feed = "all_hour"
	}
	if c.PollSeconds <= 0 {
		c.PollSeconds = 60
	}
}

func (c *Config) feedURL() string {
	return "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/" + c.Feed + ".geojson"
}

type Connector struct{ http *sdkhttp.Client }

// New returns a fresh connector.
func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }

func (c *Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "USGS Earthquakes",
		Version:     Version,
		Category:    sdk.CategoryGeophysical,
		Mode:        sdk.ModePull,
		Description: "Pulls earthquake events from USGS GeoJSON feeds.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"feed": {"type": "string", "default": "all_hour", "description": "USGS feed slug, e.g. all_hour, significant_week"},
				"pollSeconds": {"type": "integer", "minimum": 10, "default": 60},
				"minMagnitude": {"type": "number", "default": 0}
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
		return fmt.Errorf("usgs-earthquakes config: %w", err)
	}
	if cfg.PollSeconds < 0 {
		return errors.New("pollSeconds must be >= 0")
	}
	return nil
}

// feature is the slim subset of GeoJSON we care about.
type feature struct {
	ID         string `json:"id"`
	Properties struct {
		Mag     *float64 `json:"mag"`
		Place   string   `json:"place"`
		Time    int64    `json:"time"`    // ms since epoch
		Updated int64    `json:"updated"` // ms since epoch
		URL     string   `json:"url"`
		Type    string   `json:"type"` // "earthquake", "quarry blast", ...
		Tsunami int      `json:"tsunami"`
		Alert   *string  `json:"alert"` // green, yellow, orange, red, or null
		Status  string   `json:"status"`
	} `json:"properties"`
	Geometry struct {
		Coordinates []float64 `json:"coordinates"` // [lng, lat, depth_km]
	} `json:"geometry"`
}

type featureCollection struct {
	Features []feature `json:"features"`
}

// checkpointKey is the ID of the most recent event we emitted, used to dedup
// across polls. We store the last-seen timestamp instead of an ID set so the
// checkpoint stays small regardless of feed size.
const checkpointKey = "lastEventMs"

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()
	rt.Logger().Info("usgs-earthquakes starting", "feed", cfg.Feed, "pollSeconds", cfg.PollSeconds)

	// Resume from last seen event time. Zero on first run; we then accept
	// every event in the feed.
	lastSeenMs, err := loadLastSeen(ctx, rt)
	if err != nil {
		rt.Logger().Warn("load checkpoint", "err", err)
	}

	tick := time.NewTicker(time.Duration(cfg.PollSeconds) * time.Second)
	defer tick.Stop()

	// Run once immediately so the user sees data on the first poll, not
	// after `pollSeconds` of empty waiting.
	if newLast, err := c.poll(ctx, rt, cfg, lastSeenMs); err == nil {
		lastSeenMs = newLast
	} else {
		rt.Logger().Warn("initial poll", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			newLast, err := c.poll(ctx, rt, cfg, lastSeenMs)
			if err != nil {
				rt.Logger().Warn("poll", "err", err)
				continue
			}
			lastSeenMs = newLast
		}
	}
}

func (c *Connector) poll(ctx context.Context, rt sdk.Context, cfg Config, lastSeenMs int64) (int64, error) {
	body, err := c.http.GetJSON(ctx, cfg.feedURL(), nil)
	if err != nil {
		return lastSeenMs, err
	}
	var fc featureCollection
	if err := json.Unmarshal(body, &fc); err != nil {
		return lastSeenMs, fmt.Errorf("decode geojson: %w", err)
	}

	emitted := 0
	maxSeen := lastSeenMs
	for _, f := range fc.Features {
		if f.Properties.Time <= lastSeenMs {
			continue
		}
		if f.Properties.Mag != nil && *f.Properties.Mag < cfg.MinMagnitude {
			continue
		}
		if len(f.Geometry.Coordinates) < 2 {
			continue
		}

		var loc *sdk.GeoPoint
		coords := f.Geometry.Coordinates
		if len(coords) >= 2 {
			loc = &sdk.GeoPoint{Lat: coords[1], Lng: coords[0]}
			if len(coords) >= 3 {
				// Depth is in km below surface; we encode altitude as -depth_m.
				depthM := -coords[2] * 1000
				loc.Altitude = &depthM
			}
		}

		mag := 0.0
		if f.Properties.Mag != nil {
			mag = *f.Properties.Mag
		}
		alert := ""
		if f.Properties.Alert != nil {
			alert = *f.Properties.Alert
		}

		tags := map[string]string{
			"feed":     cfg.Feed,
			"type":     f.Properties.Type,
			"severity": magnitudeSeverity(mag),
			"status":   f.Properties.Status,
		}
		if alert != "" {
			tags["pager_alert"] = alert
		}
		if f.Properties.Tsunami == 1 {
			tags["tsunami"] = "1"
		}

		payload, _ := json.Marshal(map[string]any{
			"eventId":   f.ID,
			"magnitude": mag,
			"place":     f.Properties.Place,
			"url":       f.Properties.URL,
			"time":      time.UnixMilli(f.Properties.Time).UTC().Format(time.RFC3339),
			"depthKm":   depthKm(coords),
		})

		err := rt.Publish(ctx, sdk.Record{
			Timestamp: time.UnixMilli(f.Properties.Time).UTC(),
			SourceID:  f.ID,
			Location:  loc,
			Tags:      tags,
			Payload:   payload,
		})
		if err != nil {
			return maxSeen, err
		}
		emitted++
		if f.Properties.Time > maxSeen {
			maxSeen = f.Properties.Time
		}
	}

	if emitted > 0 {
		rt.Logger().Info("usgs-earthquakes emitted", "count", emitted, "feed", cfg.Feed)
		if err := rt.Checkpoint(ctx, checkpointKey, fmt.Sprintf("%d", maxSeen)); err != nil {
			rt.Logger().Warn("save checkpoint", "err", err)
		}
	}
	return maxSeen, nil
}

func loadLastSeen(ctx context.Context, rt sdk.Context) (int64, error) {
	v, err := rt.LoadCheckpoint(ctx, checkpointKey)
	if err != nil || v == "" {
		return 0, err
	}
	var ms int64
	_, err = fmt.Sscanf(v, "%d", &ms)
	return ms, err
}

func depthKm(coords []float64) float64 {
	if len(coords) >= 3 {
		return coords[2]
	}
	return 0
}

func magnitudeSeverity(mag float64) string {
	switch {
	case mag >= 6:
		return "emergency"
	case mag >= 4.5:
		return "critical"
	case mag >= 3:
		return "warning"
	default:
		return "info"
	}
}

// Ensure ID has no whitespace baked in even when imported by yaml-style tooling.
func init() {
	if strings.ContainsAny(ID, " \t\n") {
		panic("usgs-earthquakes: invalid ID")
	}
}
