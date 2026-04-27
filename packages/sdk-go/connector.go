package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Mode describes how a connector delivers records.
//
//   - ModePull:   the runtime calls Run on a schedule (cron-style polling).
//   - ModePush:   the connector exposes an HTTP handler the runtime mounts under
//     /api/ingest/<connector-id>/<route>. Records arrive via webhook.
//   - ModeStream: the connector holds a long-lived connection (MQTT, OPC-UA,
//     Kafka, websocket, ...) and publishes records as they arrive.
type Mode string

const (
	ModePull   Mode = "pull"
	ModePush   Mode = "push"
	ModeStream Mode = "stream"
)

// Category is a coarse grouping for the connector marketplace UI.
// New categories may be added as the ecosystem grows.
type Category string

const (
	CategoryGeophysical Category = "geophysical"
	CategoryWeather     Category = "weather"
	CategoryAirQuality  Category = "air_quality"
	CategoryHydrology   Category = "hydrology"
	CategoryWildfire    Category = "wildfire"
	CategoryStructural  Category = "structural"
	CategoryIoT         Category = "iot"
	CategoryIndustrial  Category = "industrial"
	CategoryCustom      Category = "custom"
)

// Manifest is connector metadata. Returned by Connector.Manifest() so the
// runtime can render UI, validate config, and route ingest paths without
// loading the connector.
type Manifest struct {
	ID           string          `json:"id"`            // "usgs-earthquakes"
	Name         string          `json:"name"`          // "USGS Earthquakes"
	Version      string          `json:"version"`       // semver
	Category     Category        `json:"category"`
	Mode         Mode            `json:"mode"`
	Description  string          `json:"description"`
	ConfigSchema json.RawMessage `json:"configSchema"` // JSON Schema draft 2020-12
}

// Record is one observation produced by a connector. Payload is opaque JSON;
// the storage layer indexes Timestamp, ConnectorID, SourceID, and Location,
// and stores Payload as a JSONB column.
type Record struct {
	Timestamp   time.Time         `json:"timestamp"`
	ConnectorID string            `json:"connectorId"`
	SourceID    string            `json:"sourceId,omitempty"` // e.g. sensor or asset id
	Location    *GeoPoint         `json:"location,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Payload     json.RawMessage   `json:"payload"`
}

// GeoPoint is WGS84 lat/lng with optional altitude in meters.
type GeoPoint struct {
	Lat      float64  `json:"lat"`
	Lng      float64  `json:"lng"`
	Altitude *float64 `json:"altitude,omitempty"`
}

// Connector is the contract every plugin implements.
//
// Lifecycle:
//  1. Runtime calls Manifest() to register the connector.
//  2. Runtime calls Validate(config) before saving config from the UI.
//  3. Runtime calls Run(ctx, config) to start the connector.
//  4. Run blocks until ctx is cancelled, then returns.
//
// Connectors must not import the storage or queue packages — they only ever
// talk to the Context they're given. This keeps plugins swappable across
// embedded (DuckDB) and scale (Redpanda + ClickHouse) deployments.
type Connector interface {
	Manifest() Manifest
	Validate(config json.RawMessage) error
	Run(ctx context.Context, rt Context, config json.RawMessage) error
}

// Context is what the runtime passes to a running connector. Connectors get
// no access to the database, queue, or other connectors — only the methods
// here. This is deliberately small in phase 0; phase 1 expands it.
type Context interface {
	// Publish hands a record to the runtime. May block under backpressure.
	Publish(ctx context.Context, r Record) error

	// Logger returns a structured logger scoped to this connector.
	Logger() Logger

	// Secret returns a named secret (e.g. API key). Returns "" if unset.
	Secret(name string) string

	// Checkpoint persists a small string the connector can use to resume
	// after a restart (e.g. last-seen event ID for pull connectors).
	Checkpoint(ctx context.Context, key, value string) error

	// LoadCheckpoint returns the last value stored under key, or "" if none.
	LoadCheckpoint(ctx context.Context, key string) (string, error)
}

// Logger is the minimal structured-logging surface connectors use.
// Implementations must be safe for concurrent use.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// PushHandler is implemented by Mode==ModePush connectors. The runtime
// mounts the returned http.Handler under /api/ingest/<instanceID>/ — the
// handler decodes the incoming request and calls Context.Publish.
//
// Connectors implement PushHandler by returning a function that constructs
// the handler from a sdk.Context. The runtime calls BuildPushHandler once
// per instance, after Run starts.
type PushHandler interface {
	BuildPushHandler(rt Context, config json.RawMessage) (http.Handler, error)
}
