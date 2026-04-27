# Changelog

All notable changes to Sunny are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and this project adheres
to [semantic versioning](https://semver.org/) starting at v1.0.

## [Unreleased]

### Added
- **MQTT stream connector.** `connectors/mqtt`. Reference implementation
  for stream-mode connectors. Subscribes to one or more topic patterns,
  publishes each message as a record. Username/password from inline
  config or `SUNNY_SECRET_MQTT_USERNAME` / `SUNNY_SECRET_MQTT_PASSWORD`.
  Auto-reconnects with backoff.
- **Export endpoint.** `GET /api/export?format=csv|parquet&connector=&from=&to=&limit=`
  streams a DuckDB COPY ... TO ... result. CSV streams directly; Parquet
  buffers to a tempfile (Parquet's not streamable). Tested for both
  formats including parquet magic-byte verification.
- **Storage `Exporter` interface.** Backends opt into export support;
  the HTTP layer falls back to 500 if a backend doesn't implement it.
- **API token auth** alongside cookie sessions. `SUNNY_API_TOKENS`
  (comma-separated, ≥16 chars each). Middleware accepts a valid cookie
  OR a valid `Authorization: Bearer <token>`. Push ingest still bypasses
  both — push connectors enforce their own tokens. `/api/auth/status`
  now returns both `enabled` (any auth factor on) and `passwordEnabled`
  (cookie/login flow available). 3 new auth tests cover token-only mode,
  short-token rejection, and cookie+token coexistence.
- **FIRMS connector tests** with a synthetic CSV fixture. 3 tests cover
  parsing, time normalization (FIRMS uses HHMM strings), and the idle-
  without-key path.
- **OpenAQ connector tests** with a hand-crafted v3 measurements
  response. 4 tests cover parsing, full poll round-trip with header
  verification, dedupe via checkpoint, and idle-without-key.
- **Sparkline component** (pure SVG, no Recharts). Each running-instance
  card on the Connectors page shows a 60-second-bucket sparkline of the
  past hour's record rate. 4 component tests.
- **`POST /api/query` endpoint.** Read-only DuckDB SQL with allowlist:
  SELECT and WITH only, multi-statement rejected, DDL/DML keywords
  rejected, hard `MaxQueryRows` cap, parameterized via `?` placeholders.
  Returns columnar `{columns, types, rows, rowCount}`. 12 tests cover
  the allowlist edge cases.
- **Postgres LISTEN/NOTIFY connector.** `connectors/postgres`. Stream-
  mode reference for change-data-capture flows. Each NOTIFY arriving on
  a configured channel becomes a record. DSN-based connection;
  `SUNNY_SECRET_POSTGRES_USERNAME` / `SUNNY_SECRET_POSTGRES_PASSWORD`
  override DSN credentials. Auto-reconnects with backoff. 4 tests cover
  validate, manifest, identifier quoting, and credential overrides.
- **Per-IP rate limit** on `/api/query` and `/api/export`. Token-bucket,
  10 rpm by default, configurable via `SUNNY_QUERY_RPM`. Returns 429
  with `Retry-After` header. 4 unit tests; live-tested 12 rapid
  requests → first 9 succeed, requests 10–12 → 429.
- **Per-instance metrics endpoint** `GET /api/connectors/{id}/metrics`.
  Returns state, restarts, total records, last record timestamp, rate
  per minute over the last hour and last 24 h. Useful for grafana-style
  scrapers. 2 tests.
- **`sunny-cli query`** subcommand. POSTs SQL to `/api/query`, renders
  the result as an ASCII table on stdout. Honors `SUNNY_SERVER` and
  `SUNNY_TOKEN` env vars; `--server` and `--token` flags override.
- **`/api/backup` endpoint.** Streams a gzipped tarball of the data
  directory. Tighter rate limit (2 rpm). Auth-protected. The offline
  `sunny-cli backup` is still preferred for strictly consistent
  snapshots; this is for cases without shell access.
- **CORS support.** `SUNNY_CORS_ORIGINS` (comma-separated origins, or
  `*`). Adds standard `Access-Control-*` headers + handles preflight
  OPTIONS. Required for browser-based third-party UIs and Grafana-
  style integrations that scrape from the browser. 5 unit tests.
- **Structured access log middleware.** Every API request logs as a
  JSON line: method, path, status, duration_ms, bytes, remote IP,
  request_id. Skips `/api/health`, `/api/version`, and `/assets/*` to
  keep volume sane. Status-aware level (INFO/WARN/ERROR), `slow=true`
  attribute for >500ms requests. 3 unit tests.
- **`GET /api/connectors/instances`** — lighter version of
  `/api/connectors` returning just the running-instance array. The
  sidebar polls this every 5 s; saves bandwidth on each tick.
- **`sunny-cli watch`** subcommand. Opens `WS /api/stream`, prints
  records as they arrive in `timestamp connector headline` format.
  Optional `--connector ID` to filter server-side.
- **`/healthz` and `/readyz`** Kubernetes-style probes. Public, outside
  `/api` so they bypass auth, CORS, and rate limits. `/readyz` returns
  503 if storage is unreachable or any instance is in `failed` state.
  Helm chart already wired to use these in livenessProbe/readinessProbe.
- **`sunny-cli connectors`** subcommand. `connectors instances` lists
  running instances as a table (INSTANCE/TYPE/STATE/RESTARTS/AGE);
  `connectors types` lists registered types (ID/NAME/MODE/CATEGORY/VERSION).
- **CSRF protection on `/api/auth/login`.** Rejects cross-origin browser
  POSTs (Origin or Referer mismatch with Host) with 403. Same-origin
  posts pass; API tools without an Origin header pass (token middleware
  handles those cases anyway). Live-tested across 4 scenarios.
- **OpenAPI 3.1 spec** at `docs/openapi.yaml`. Embedded via `go:embed`
  and served at `GET /api/openapi.yaml`. `GET /api/docs` renders a
  RapiDoc-based interactive viewer pointing at the spec. Covers every
  public endpoint with request/response schemas + examples. 2 tests.
- **WebSocket connection cap** on `/api/stream`. Default 200 concurrent;
  configurable via `SUNNY_MAX_STREAM_CONNS`. Returns 503 + Retry-After
  on overflow. 1 test.
- **Connectors page integration test.** 7 tests covering tile rendering,
  category grouping, instance cards, push-mode UI surfacing, restart
  warnings, modal opening, and the curl example for push connectors.
- **Dashboard empty-state banner.** Shows "Connectors are starting up"
  with a CLI hint for the first 30 s of a fresh boot, until records
  arrive or the timeout elapses.
- **Dashboard chunk slimmed** from 379 KB → 14.7 KB. Recharts now lives
  in its own lazy-loaded `DashboardCharts` chunk (366 KB) that arrives
  after the dashboard skeleton renders. The two charts now live in
  `apps/web/src/components/charts/DashboardCharts.tsx`.

## [0.1.0] — 2026-04-26

First public pre-release. Single binary, embedded frontend, real
connectors, real persistence, real alerts, single-password auth.

### Platform

- **Single Go binary** with the React frontend embedded via `go:embed`.
  Serves API + SPA on one port.
- **DuckDB-backed persistence.** Wide events table + checkpoints + alerts
  + alert rules. Survives restarts. Backup/restore via CLI.
- **In-process bus** with fan-out fanout + per-subscriber drop counters.
- **WebSocket live tail** at `/api/stream` with optional connector filter
  and ring-buffer replay.

### Connector SDK

- `Connector` interface with three modes: pull, push, stream.
- `Context` exposes `Publish`, `Logger`, `Secret`, `Checkpoint`,
  `LoadCheckpoint`. Push-mode connectors implement `BuildPushHandler`.
- Side-effect-import registration pattern (`builtins/builtins.go`).
- Shared HTTP client (`sdkhttp`) with retries on 429/5xx and per-attempt
  backoff.

### First-party connectors

- `usgs-earthquakes` — USGS GeoJSON earthquake feeds. No auth.
- `noaa-weather-alerts` — NWS active alerts. No auth.
- `usgs-water` — USGS NWIS instantaneous values. No auth.
- `nasa-firms` — VIIRS/MODIS active-fire detections. Free MAP_KEY required.
- `openaq` — OpenAQ v3 air-quality measurements. Free API key required.
- `webhook` — Generic JSON push receiver with header-driven tags.
- `hello` — Heartbeat connector for development.

### HTTP API

- `GET /api/health`, `/api/version`, `/api/auth/{status,login,logout}`.
- `GET /api/connectors`, `/api/connectors/{id}`, `/api/connectors/registry`.
- `GET /api/records`, `/api/records/recent`, `/api/records/counts`.
- `GET /api/timeseries` with DuckDB-side bucketing.
- `GET /api/alerts`, `POST /api/alerts/{id}/ack`.
- `GET /api/alerts/rules`, `POST /api/alerts/rules`,
  `DELETE /api/alerts/rules/{id}`.
- `WS /api/stream` with `?connector=` and `?replay=`.
- `POST /api/ingest/{instance-id}/...` for push connectors.

### Frontend

- Dashboard with live ingest throughput chart (server-aggregated),
  severity distribution, critical record feed, ingest pipeline view.
- LiveMap (Leaflet) with per-connector toggleable layers.
- DataStreams: per-instance state, restart count, live record tail.
- Alerts page with severity chips, search, ack button.
- Connectors marketplace: registered manifests grouped by category, push
  endpoint display with copy button, registry-merged metadata, schema
  preview, install instructions.
- AuthGate: login screen when `SUNNY_PASSWORD_HASH` is set.
- Responsive layout with a mobile drawer at < 900px.
- React.lazy code-splitting: initial bundle ~75KB gzipped, Recharts +
  Leaflet load only on the pages that need them.

### Auth

- Single-password (bcrypt) auth, opt-in via `SUNNY_PASSWORD_HASH`.
- HMAC-signed cookie sessions, 7-day TTL.
- `SUNNY_SESSION_KEY` honored if set; random per-startup otherwise.
- API surfaces under `/api` are protected; `/api/health`, `/api/version`,
  and `/api/auth/*` stay public.

### CLI (`sunny-cli`)

- `version` — print the CLI version.
- `hash-password <pw>` — generate a bcrypt hash for `SUNNY_PASSWORD_HASH`.
- `backup <data-dir> <out.tar.gz>` — gzipped tar snapshot, skips WAL.
- `restore <in.tar.gz> <data-dir>` — refuses to overwrite a non-empty dir.

### Self-host

- `Dockerfile` (multi-stage, cgo, alpine final, non-root, healthcheck).
- `docker-compose.yml` with persistent volume.
- `scripts/install.sh` — OS/arch detection, downloads release.
- Helm chart (`charts/sunny/`) — Deployment, Service, PVC, Secret,
  optional Ingress.
- GitHub Actions: `ci.yml` (test + type-check + bundle smoke),
  `release.yml` (cross-compile binaries, build & push Docker, attach
  release artifacts).

### Docs

- `README.md` — install, what's bundled, configuration, dev cycle.
- `CONTRIBUTING.md` — repo layout, dev cycle, "how to add a connector".
- `docs/connectors/tutorial.md` — 15-minute walkthrough.
- `docs/faq.md` — common questions.
- `docs/semver.md` — what's stable.
- `SECURITY.md` — disclosure policy + threat model.
- `docs/connector-registry-schema.json` + `docs/registry.example.json`.

### Tests

- ~30 backend Go tests (bus, runtime, storage, alerts, auth, config,
  connectors, registry).
- ~10 connector parser tests against captured fixtures.
- 34 frontend Vitest tests (API client, AuthGate phases, formatters).

[Unreleased]: https://github.com/sunny/sunny/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/sunny/sunny/releases/tag/v0.1.0
