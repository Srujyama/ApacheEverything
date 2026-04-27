# Build a Sunny connector in 15 minutes

You're going to write a connector that pulls earthquake data from the USGS
GeoJSON feed and publishes one record per event into Sunny's bus. By the end
you'll see real earthquakes show up on the LiveMap.

If you finish faster than 15 minutes, write us a tougher tutorial.

## What a connector is

A connector is a Go package that implements one interface:

```go
type Connector interface {
    Manifest() Manifest
    Validate(config json.RawMessage) error
    Run(ctx context.Context, rt Context, config json.RawMessage) error
}
```

`Manifest` is metadata. `Validate` rejects bad config before the connector
runs. `Run` does the work — it's called in its own goroutine and runs until
ctx is cancelled. Connectors get no direct access to the database or queue;
the runtime hands them a small `Context` with `Publish`, `Logger`,
`Secret`, and `Checkpoint` methods.

There are three modes:

- **Pull**: you tick on a schedule, fetch from an API, publish records.
- **Push**: the runtime mounts your HTTP handler at
  `/api/ingest/<id>/<route>`. Records arrive as webhooks.
- **Stream**: you hold a long-lived connection (MQTT, OPC-UA, websocket)
  and publish records as they arrive.

This tutorial uses Pull mode.

## Step 1 — Scaffolding

Create `connectors/myquakes/connector.go`:

```go
package myquakes

import (
    "context"
    "encoding/json"
    "time"

    sdk "github.com/sunny/sunny/packages/sdk-go"
    "github.com/sunny/sunny/packages/sdk-go/sdkhttp"
)

const (
    ID      = "myquakes"
    Version = "0.1.0"
)

type Connector struct{ http *sdkhttp.Client }

// New is what builtins.go imports and registers.
func New() sdk.Connector { return &Connector{http: sdkhttp.New()} }
```

`sdkhttp.Client` is the shared HTTP client connectors use — handles retries,
backoff, sane User-Agent. Use it instead of stdlib `http.Get`.

## Step 2 — Manifest

`Manifest` populates the marketplace UI and tells the runtime what config
your connector accepts:

```go
func (c *Connector) Manifest() sdk.Manifest {
    return sdk.Manifest{
        ID:          ID,
        Name:        "My Quakes",
        Version:     Version,
        Category:    sdk.CategoryGeophysical,
        Mode:        sdk.ModePull,
        Description: "USGS earthquakes (tutorial connector).",
        ConfigSchema: json.RawMessage(`{
            "type": "object",
            "properties": {
                "pollSeconds": {"type": "integer", "minimum": 10, "default": 60}
            }
        }`),
    }
}
```

The `configSchema` is rendered into the auto-generated config form on the
Connectors page. JSON Schema draft 2020-12; `default` and `description`
both surface to the user.

## Step 3 — Config + Validate

```go
type Config struct {
    PollSeconds int `json:"pollSeconds"`
}

func (c *Config) applyDefaults() {
    if c.PollSeconds <= 0 { c.PollSeconds = 60 }
}

func (Connector) Validate(raw json.RawMessage) error {
    if len(raw) == 0 { return nil }
    var cfg Config
    return json.Unmarshal(raw, &cfg)
}
```

`Validate` runs every time the user saves config from the UI. Reject
nonsensical values here — don't wait until `Run`.

## Step 4 — Run

This is where the work happens. The contract:

- Fetch on a tick.
- For each new event, call `rt.Publish(ctx, Record{...})`.
- Use `rt.Checkpoint` to remember where you left off — survives restarts.
- Return only when `ctx.Done()`. Errors trigger the supervisor's
  exponential backoff restart.

```go
const checkpointKey = "lastEventMs"

type feature struct {
    ID         string
    Properties struct {
        Mag   *float64
        Place string
        Time  int64
    }
    Geometry struct {
        Coordinates []float64 // [lng, lat, depth_km]
    }
}
type response struct{ Features []feature }

func (c *Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
    cfg := Config{}
    if len(raw) > 0 { _ = json.Unmarshal(raw, &cfg) }
    cfg.applyDefaults()

    rt.Logger().Info("myquakes starting", "pollSeconds", cfg.PollSeconds)

    var lastSeen int64
    if v, _ := rt.LoadCheckpoint(ctx, checkpointKey); v != "" {
        _, _ = fmt.Sscanf(v, "%d", &lastSeen)
    }

    tick := time.NewTicker(time.Duration(cfg.PollSeconds) * time.Second)
    defer tick.Stop()
    poll := func() {
        body, err := c.http.GetJSON(ctx,
            "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/all_hour.geojson",
            nil,
        )
        if err != nil { rt.Logger().Warn("poll", "err", err); return }
        var resp response
        if err := json.Unmarshal(body, &resp); err != nil { return }

        max := lastSeen
        for _, f := range resp.Features {
            if f.Properties.Time <= lastSeen { continue }
            mag := 0.0
            if f.Properties.Mag != nil { mag = *f.Properties.Mag }
            payload, _ := json.Marshal(map[string]any{
                "magnitude": mag,
                "place":     f.Properties.Place,
            })
            _ = rt.Publish(ctx, sdk.Record{
                Timestamp: time.UnixMilli(f.Properties.Time).UTC(),
                SourceID:  f.ID,
                Location: &sdk.GeoPoint{
                    Lat: f.Geometry.Coordinates[1],
                    Lng: f.Geometry.Coordinates[0],
                },
                Tags:    map[string]string{"severity": severityFor(mag)},
                Payload: payload,
            })
            if f.Properties.Time > max { max = f.Properties.Time }
        }
        if max > lastSeen {
            _ = rt.Checkpoint(ctx, checkpointKey, fmt.Sprintf("%d", max))
            lastSeen = max
        }
    }

    poll() // run once immediately
    for {
        select {
        case <-ctx.Done(): return ctx.Err()
        case <-tick.C: poll()
        }
    }
}

func severityFor(mag float64) string {
    switch {
    case mag >= 6: return "emergency"
    case mag >= 4.5: return "critical"
    case mag >= 3: return "warning"
    default: return "info"
    }
}
```

## Step 5 — Register

Edit `apps/server/internal/connectors/builtins/builtins.go`:

```go
import myquakes "github.com/sunny/sunny/connectors/myquakes"

func init() {
    // ...existing registrations...
    connectors.Register(myquakes.New())
}
```

## Step 6 — Run it

```sh
cd apps/server && go run ./cmd/sunny
```

In another terminal:

```sh
curl 'http://localhost:3000/api/connectors' | jq '.types[] | select(.id=="myquakes")'
curl 'http://localhost:3000/api/records?connector=myquakes&limit=3'
```

Add it to your `sunny.config.yaml`:

```yaml
connectors:
  - id: my-quakes
    type: myquakes
    config: { pollSeconds: 30 }
```

## What you just built

You've got: a pull-mode connector with config validation, retry-aware HTTP,
checkpoint resume, severity tagging, geographic data. That's the same
shape every first-party Sunny connector takes — `usgsearthquakes`,
`noaaweather`, `usgswater` are 80% the same code with different parsers.

## Going further

- **Push mode**: implement `RegisterPushHandler` on your connector. The
  runtime mounts it at `/api/ingest/<id>/`. See `connectors/webhook/`.
- **Stream mode**: open your connection in `Run`, loop on incoming
  messages, publish each. Be cancel-aware.
- **Secrets**: `rt.Secret("my-key")` reads `SUNNY_SECRET_MY_KEY`. Idle
  gracefully if missing — see `connectors/nasafirms/` for the pattern.
- **Tags vs Payload**: tags are queryable (the alert engine filters on
  them); payload is opaque JSON. Put dimensions in tags, values in payload.

## Submitting a connector

See [CONTRIBUTING.md](../../CONTRIBUTING.md). Connector PRs are reviewed
within the week if they include a smoke test and a config-schema doc string.
