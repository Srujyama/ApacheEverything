// Package usgswater is a Sunny connector for USGS Water Services
// (https://waterservices.usgs.gov). It fetches gauge-height readings (and
// optionally other instantaneous-value parameters) for a configured list of
// monitoring sites. No auth required.
package usgswater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
	"github.com/sunny/sunny/packages/sdk-go/sdkhttp"
)

const (
	ID      = "usgs-water"
	Version = "0.1.0"
)

// DefaultSites is a curated set of major US river/reservoir gauges that
// gives a fresh installer something interesting on first boot.
var DefaultSites = []string{
	"09421500", // Colorado River below Hoover Dam, AZ-NV
	"14211720", // Willamette River at Portland, OR
	"03612600", // Ohio River at Metropolis, IL
	"08074500", // Buffalo Bayou at Houston, TX
	"01646500", // Potomac River near Washington, DC
	"02035000", // James River at Cartersville, VA
	"06892350", // Kansas River at DeSoto, KS
	"05587450", // Mississippi River below Grafton, IL
}

type Config struct {
	// Sites is the list of USGS site codes to query. Defaults to a curated
	// list of major rivers/reservoirs.
	Sites []string `json:"sites"`

	// ParameterCD is the USGS parameter code. 00065 = gauge height (ft),
	// 00060 = streamflow (cfs), 00010 = water temperature.
	ParameterCD string `json:"parameterCd"`

	// PollMinutes is how often to poll. Default 15 (USGS IV updates ~every 15min).
	PollMinutes int `json:"pollMinutes"`
}

func (c *Config) applyDefaults() {
	if len(c.Sites) == 0 {
		c.Sites = append(c.Sites, DefaultSites...)
	}
	if c.ParameterCD == "" {
		c.ParameterCD = "00065"
	}
	if c.PollMinutes <= 0 {
		c.PollMinutes = 15
	}
}

func (c *Config) feedURL() string {
	return fmt.Sprintf(
		"https://waterservices.usgs.gov/nwis/iv/?format=json&sites=%s&parameterCd=%s",
		strings.Join(c.Sites, ","), c.ParameterCD,
	)
}

type Connector struct{ http *sdkhttp.Client }

func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }

func (c *Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "USGS Water Services",
		Version:     Version,
		Category:    sdk.CategoryHydrology,
		Mode:        sdk.ModePull,
		Description: "Pulls gauge readings from USGS Water Services (NWIS IV).",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"sites": {"type": "array", "items": {"type": "string"}},
				"parameterCd": {"type": "string", "default": "00065"},
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
		return fmt.Errorf("usgs-water config: %w", err)
	}
	if cfg.PollMinutes < 0 {
		return errors.New("pollMinutes must be >= 0")
	}
	return nil
}

// nwisResponse is the minimal subset of the NWIS IV response we use.
type nwisResponse struct {
	Value struct {
		TimeSeries []struct {
			SourceInfo struct {
				SiteName string `json:"siteName"`
				SiteCode []struct {
					Value string `json:"value"`
				} `json:"siteCode"`
				GeoLocation struct {
					Geog struct {
						Lat float64 `json:"latitude"`
						Lng float64 `json:"longitude"`
					} `json:"geogLocation"`
				} `json:"geoLocation"`
			} `json:"sourceInfo"`
			Variable struct {
				VariableName string `json:"variableName"`
				VariableCode []struct {
					Value string `json:"value"`
				} `json:"variableCode"`
				Unit struct {
					UnitCode string `json:"unitCode"`
				} `json:"unit"`
			} `json:"variable"`
			Values []struct {
				Value []struct {
					Value    string `json:"value"`
					DateTime string `json:"dateTime"`
				} `json:"value"`
			} `json:"values"`
		} `json:"timeSeries"`
	} `json:"value"`
}

const checkpointKey = "lastSeenIso"

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()
	rt.Logger().Info("usgs-water starting", "sites", len(cfg.Sites), "pollMinutes", cfg.PollMinutes)

	lastSeen, _ := rt.LoadCheckpoint(ctx, checkpointKey)

	tick := time.NewTicker(time.Duration(cfg.PollMinutes) * time.Minute)
	defer tick.Stop()

	lastSeen = c.pollOnce(ctx, rt, cfg, lastSeen)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			lastSeen = c.pollOnce(ctx, rt, cfg, lastSeen)
		}
	}
}

func (c *Connector) pollOnce(ctx context.Context, rt sdk.Context, cfg Config, lastSeen string) string {
	body, err := c.http.GetJSON(ctx, cfg.feedURL(), nil)
	if err != nil {
		rt.Logger().Warn("poll", "err", err)
		return lastSeen
	}
	var resp nwisResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		rt.Logger().Warn("decode", "err", err)
		return lastSeen
	}

	emitted := 0
	maxSeen := lastSeen
	for _, ts := range resp.Value.TimeSeries {
		siteCode := ""
		if len(ts.SourceInfo.SiteCode) > 0 {
			siteCode = ts.SourceInfo.SiteCode[0].Value
		}
		varCode := ""
		if len(ts.Variable.VariableCode) > 0 {
			varCode = ts.Variable.VariableCode[0].Value
		}
		for _, vs := range ts.Values {
			for _, v := range vs.Value {
				dt, err := time.Parse(time.RFC3339, v.DateTime)
				if err != nil {
					// NWIS sometimes uses non-Z offsets; the parse handles those.
					continue
				}
				key := siteCode + "|" + varCode + "|" + v.DateTime
				if key <= lastSeen {
					continue
				}
				val, err := strconv.ParseFloat(v.Value, 64)
				if err != nil {
					continue
				}

				tags := map[string]string{
					"site":         siteCode,
					"siteName":     ts.SourceInfo.SiteName,
					"variableCode": varCode,
					"unit":         ts.Variable.Unit.UnitCode,
				}
				payload, _ := json.Marshal(map[string]any{
					"value":        val,
					"unit":         ts.Variable.Unit.UnitCode,
					"variableName": ts.Variable.VariableName,
				})

				err = rt.Publish(ctx, sdk.Record{
					Timestamp: dt.UTC(),
					SourceID:  siteCode,
					Location:  &sdk.GeoPoint{Lat: ts.SourceInfo.GeoLocation.Geog.Lat, Lng: ts.SourceInfo.GeoLocation.Geog.Lng},
					Tags:      tags,
					Payload:   payload,
				})
				if err != nil {
					rt.Logger().Warn("publish", "err", err)
					return maxSeen
				}
				emitted++
				if key > maxSeen {
					maxSeen = key
				}
			}
		}
	}

	if emitted > 0 {
		rt.Logger().Info("usgs-water emitted", "count", emitted)
		_ = rt.Checkpoint(ctx, checkpointKey, maxSeen)
	}
	return maxSeen
}
