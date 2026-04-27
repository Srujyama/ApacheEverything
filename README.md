# Sunny

**Open-source, self-hosted observability for physical infrastructure.**

Sunny is what n8n is to Zapier — but for physical-world data. One Go
binary, an embedded React dashboard, a connector SDK, DuckDB on disk.
Plug in real public-data feeds (USGS earthquakes, NOAA weather alerts,
USGS river gauges, NASA wildfires, OpenAQ air quality) or your own MQTT,
webhooks, or Postgres CDC stream.

Run it on your laptop, on a $5 VPS, or on Kubernetes. Own your data.

> **Status:** v0.1 pre-release. The ingest pipeline, connector SDK,
> dashboard, alert engine, auth, and CLI are all real and tested. We're
> closing in on v1.0.

## 30-second pitch

```sh
# Pull & run. No config needed — boots with real public-data connectors on.
docker run -p 3000:3000 -v sunny-data:/data ghcr.io/sunny/sunny:latest
```

Open <http://localhost:3000> and within a minute you have a live map of
real U.S. earthquakes, active weather alerts, and major river gauge
readings, plus a connector marketplace, an alert rule engine, and a
WebSocket-fed live record stream.

## Why Sunny

- **One binary, one port.** The Go server embeds the React frontend; no
  reverse proxy required. ~64 MB.
- **DuckDB on disk.** Single-writer, single-file, columnar. Backups are
  just `cp`. Query history with SQL or the timeseries API.
- **Connectors are the product.** A 30-line Go file becomes a
  first-class plugin. Three modes: pull, push, stream.
- **Self-hosted by design.** No SaaS, no required cloud account, no
  telemetry. Single-password auth ships in the box.
- **n8n-style ergonomics.** A registry, an admin CLI, a Helm chart, a
  Docker image, an install script.

## Install

### Docker (recommended)

```sh
docker run -p 3000:3000 -v sunny-data:/data ghcr.io/sunny/sunny:latest
```

Or [`docker compose up`](./docker-compose.yml) from the repo root.

### Binary

```sh
curl -fsSL https://get.sunny.dev/install.sh | sh
sunny                  # serves on :3000
```

### Helm

```sh
helm install sunny ./charts/sunny
```

See [`charts/sunny/values.yaml`](./charts/sunny/values.yaml) for tunables.

## What it ships with

| Connector | Mode | What it does | Auth |
| --- | --- | --- | --- |
| `usgs-earthquakes` | pull | USGS GeoJSON earthquake feeds | none |
| `noaa-weather-alerts` | pull | NWS active alerts across the US | none |
| `usgs-water` | pull | USGS NWIS gauge readings on major rivers | none |
| `nasa-firms` | pull | NASA active-fire detections (VIIRS/MODIS) | free MAP_KEY |
| `openaq` | pull | OpenAQ v3 air-quality measurements | free API key |
| `webhook` | push | Generic webhook receiver with header-driven tags | optional token |
| `mqtt` | stream | MQTT broker subscriber. Reference impl for stream mode | optional |
| `postgres` | stream | Postgres LISTEN/NOTIFY for change-data-capture | DSN or secrets |
| `hello` | pull | Heartbeat for development | none |

[Build a connector in 15 minutes →](./docs/connectors/tutorial.md)

## Repo layout

```
apps/
  web/            React + Vite frontend (TypeScript)
  server/         Go server: HTTP API, ingest, storage, alerts
                  embeds apps/web/dist via go:embed at build time

packages/
  sdk-go/         Connector SDK (Go) — the canonical plugin contract
  sdk-ts/         Connector SDK (TypeScript) — RPC wrapper for out-of-process
  cli/            sunny-cli admin tool (hash-password, backup, restore)
  core/           Shared TypeScript types

connectors/       First-party connectors (USGS, NOAA, FIRMS, OpenAQ, …)

charts/sunny/     Helm chart
docs/             User docs, schema, tutorial, FAQ
scripts/          install.sh
```

## Configuration

Sunny reads `sunny.config.yaml` from `$SUNNY_DATA_DIR` (default `./data/`).
With no config it boots the no-auth defaults — see
[`sunny.config.example.yaml`](./sunny.config.example.yaml).

Environment overrides:

| Variable | Default | Meaning |
| --- | --- | --- |
| `SUNNY_ADDR` | `:3000` | listen address |
| `SUNNY_DATA_DIR` | `./data` | DuckDB + checkpoint storage |
| `SUNNY_PASSWORD_HASH` | unset | bcrypt; enables single-password auth |
| `SUNNY_SESSION_KEY` | random | HMAC key for session cookies |
| `SUNNY_SECRET_NASA_FIRMS_KEY` | unset | enables nasa-firms |
| `SUNNY_SECRET_OPENAQ_API_KEY` | unset | enables openaq |

To turn on auth:

```sh
sunny-cli hash-password 'your-pw'
# copy the output into SUNNY_PASSWORD_HASH and restart
```

## Development

You need Go 1.25+, Node 20+, pnpm 10+, Docker (optional).

```sh
pnpm install
cd apps/server && go mod download

# in two terminals
cd apps/server && go run ./cmd/sunny             # backend on :3000
pnpm --filter @sunny/web dev                     # frontend with HMR on :5173
```

Production-shape build:

```sh
pnpm --filter @sunny/web build
cp -R apps/web/dist/. apps/server/internal/web/dist/
cd apps/server && go build -o ../../bin/sunny ./cmd/sunny
./bin/sunny                                       # one binary, one port
```

Tests:

```sh
cd apps/server && go test ./...    # ~30 backend tests
cd connectors && go test ./...     # connector parser/poll tests
cd apps/web && pnpm test           # 34 frontend tests
```

## License

[SSPL v1](./LICENSE). Same license as MongoDB and Elastic. You can
self-host freely; you cannot offer Sunny as a managed service without
open-sourcing your entire service stack.

## Docs

- [FAQ](./docs/faq.md)
- [Connector tutorial](./docs/connectors/tutorial.md)
- [Versioning policy](./docs/semver.md)
- [Security policy](./SECURITY.md)
- [Contributing](./CONTRIBUTING.md)
