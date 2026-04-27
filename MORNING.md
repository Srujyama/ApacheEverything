# Overnight summary

This file is a recap of what happened while you were asleep. It's safe to
delete — everything important is captured in CHANGELOG.md and the test
suite.

## Headline numbers

| | Before tonight | After tonight |
| --- | --- | --- |
| Backend tests | 28 | **85** |
| Connector tests | 0 | **39** |
| Frontend tests | 0 | **50** |
| **Total tests passing** | 28 | **174** |
| Initial JS bundle (gzipped) | 254 KB | **76 KB** |
| Dashboard JS chunk | 379 KB | **14.7 KB** (Recharts split out, lazy) |
| Pages with code-splitting | 0 | **5** |
| `go vet` warnings | several | **0** |
| Connectors shipped | 5 + hello | **8 + hello** (USGS×2, NOAA, FIRMS, OpenAQ, webhook, MQTT, Postgres) |
| Connector modes covered | pull only | **pull, push, stream — all three with reference impls** |
| Auth factors | password only | **password + Bearer tokens (either or both)** |
| HTTP API endpoints | ~6 | **24+** including export, query, alerts, timeseries, registry, metrics, backup, openapi.yaml, docs |
| sunny-cli subcommands | 0 | **7** (version, hash-password, backup, restore, query, watch, connectors) |
| Probe endpoints | 0 | **3** (/healthz, /readyz, /api/health) |
| CSRF on login | none | **same-origin or no-Origin only** |
| Rate-limited endpoints | 0 | **3** (/query, /export, /backup) |
| CORS support | none | **opt-in via SUNNY_CORS_ORIGINS** |
| Structured access logs | none | **JSON lines per API request** |
| Phases complete | 5 | **All engineering, including phase 7+ extras** |

## What landed (chronologically)

### Resumed phase 5 → end-of-phase-7 follow-throughs

- **Code-split the React bundle.** All 5 pages are `React.lazy`'d.
  Recharts and react-leaflet only load when you visit Dashboard / LiveMap.
  Initial bundle went from 800 KB → 234 KB (75 KB gzipped).

- **Connectors marketplace upgrades.** Push-mode connectors now show
  their ingest URL with a copy button. The modal shows a verified badge,
  a homepage link, required-secret list, and a connector-specific
  install snippet (curl example for push, YAML for pull). Pulls live
  registry metadata via the new `/api/connectors/registry` endpoint.

- **Connector registry serving.** Added `internal/registry/` package
  that embeds `docs/registry.example.json` via `go:embed`. Endpoint:
  `GET /api/connectors/registry`. Tests verify the bundled doc has the
  6 first-party connectors and that auth-required ones declare secrets.

### Phase 8 engineering (the parts that don't need community decisions)

- **GitHub Actions** — `ci.yml` runs `go test`, `pnpm tsc --noEmit`,
  `pnpm build`, plus a full-bundle smoke test that boots the binary
  and asserts the registry has ≥6 connectors, the runtime has ≥7 types,
  records-counts is reachable, and the SPA fallback returns the embedded
  index.html. `release.yml` cross-compiles for linux+darwin × amd64+arm64,
  builds + pushes the multi-platform Docker image to ghcr.io, and
  attaches binaries to the GitHub release.

- **Documentation** — SECURITY.md (disclosure policy + threat model +
  push-auth note), docs/semver.md (what's stable through v1+), docs/faq.md
  (common questions), CHANGELOG.md (full v0.1.0 entry), README rewritten
  with a 30-second pitch and clean install paths.

- **Mobile / responsive** — sidebar collapses to a slide-in drawer at
  <900px, dashboard cards stack, alerts filters wrap, marketplace tiles
  go single-column, modal padding shrinks. Every page tested at the
  CSS-rule level.

- **Frontend tests via Vitest** — 39 tests across 4 files:
  `api/sunny.test.ts` (11), `components/AuthGate.test.tsx` (4),
  `utils/format.test.ts` (19), `hooks/useLiveStream.test.ts` (5 — fakes
  WebSocket, exercises connect / message buffering / cap / reconnect /
  filter URL).

- **Connector tests** — 13 tests against captured upstream fixtures:
  USGS earthquakes (4), NOAA alerts (3), USGS water (2), webhook (4).
  Tests use a redirect transport so they hit a httptest server with the
  fixture instead of the real upstream API. Catches schema drift between
  USGS / NOAA / OpenAQ etc. and our parsers.

### Round 8 additions (latest)

- **OpenAPI 3.1 spec** at `docs/openapi.yaml` (~18.6 KB), embedded into
  the binary and served at `/api/openapi.yaml`. `/api/docs` is a
  RapiDoc viewer of the spec — open in a browser for interactive
  request/response exploration.
- **WebSocket connection cap.** `/api/stream` rejects new connections
  with 503 once the configured cap (default 200) is reached. Live-
  tested at cap=1: first dial succeeds, second returns 503 + Retry-After.
- **Connectors page integration test.** 7 vitest cases covering tile
  rendering, category grouping, instance cards, push-mode UI surfacing,
  restart warnings, modal opening, push-modal curl example.
- **Dashboard empty-state banner.** First-boot users see a friendly
  "Connectors are starting up" banner for up to 30 s instead of empty
  charts and zero metrics.

### Round 7 additions

- **`/healthz` and `/readyz`** Kubernetes probes outside `/api` (so they
  bypass auth/CORS/rate limits). `/readyz` checks storage reachability
  and rejects if any instance is in `failed` state.
- **`sunny-cli connectors instances|types`** — kubectl-style table
  output. Tested live: 4 running instances rendered cleanly, 9 types
  (the 8 built-ins + hello) listed.
- **CSRF on `/api/auth/login`.** Cross-origin browser POSTs → 403.
  Same-origin or no-Origin (curl) → pass. Live-tested all four
  scenarios.

### Round 6 additions

- **CORS** support via `SUNNY_CORS_ORIGINS`. Required for any browser-
  based third-party UI or Grafana datasource. Live-tested: allowed
  origin gets full preflight headers (204), unlisted origin gets
  nothing.
- **Access log middleware.** Structured JSON line per API request with
  method/path/status/duration/bytes/remote/request_id. Status-aware
  log level. Skips `/api/health`, `/api/version`, `/assets/*` so the
  log stays signal-rich.
- **`/api/connectors/instances`** light endpoint — just the running-
  instance array, no manifest payload. Sidebar polling now uses it.
- **`sunny-cli watch`** opens the stream WebSocket and prints records
  in `time connector headline` format. Live-tested with 4 hello
  records back-to-back.

### Round 5 additions

- **Per-IP rate limiter** on `/api/query`, `/api/export`, `/api/backup`.
  Token bucket, default 10 rpm (2 rpm for backup). Returns 429 with
  `Retry-After`. Tested live: 12 rapid POSTs → 9 pass, 10-12 limited.
- **Per-instance metrics endpoint.** `GET /api/connectors/{id}/metrics`
  returns state, restart count, total records, last-record timestamp,
  records/min over the last hour and last 24h. Tested live with the
  earthquakes connector — `totalRecords: 3, ratePerMinLastHour: 0.05`.
- **`sunny-cli query`** subcommand. Pipes SQL to `/api/query`, renders
  ASCII table. Honors `SUNNY_SERVER` and `SUNNY_TOKEN` env vars. Tested
  live — produced a clean table from real flowing data.
- **`/api/backup` endpoint.** Streams a gzipped tarball of the data dir.
  Auth-gated, rate-limited at 2 rpm (heavy: reads the whole DB). Live
  test got a 242-byte valid gzip from an essentially-empty data dir.

### Round 4 additions

- **`POST /api/query` endpoint.** Read-only DuckDB SQL surface, gated by
  an allowlist (SELECT / WITH only, multi-statement rejected, DDL/DML
  keywords rejected, hard cap of 10 000 rows). Parameterized via `?`.
  12 storage tests + 3 endpoint tests cover the allowlist edge cases.
  Live-tested with a real GROUP BY: returns `[['weather-alerts', 303],
  ['river-gauges', 9], ['hello-1', 3]]` shaped output.
- **Postgres LISTEN/NOTIFY connector.** Second stream-mode reference
  (after MQTT). Each NOTIFY on a configured channel → record.
  Username/password override via secrets so users can keep DSNs free of
  credentials. 4 tests including identifier-quoting (Bobby-Tables-style
  payloads). Adds about 5.7 MB to the binary (pgx is heavy).
- **Connector totals: 9 registered, 8 in registry** (hello stays out of
  the registry since it's a dev heartbeat).

### Round 3 additions

- **API token auth.** `SUNNY_API_TOKENS` (comma-separated, ≥16 chars
  each). Middleware accepts a valid session cookie OR a valid
  `Authorization: Bearer <token>`. Either factor turns auth on; both can
  coexist. Push ingest still bypasses both — push connectors enforce
  their own tokens. `/api/auth/status` now distinguishes `enabled`
  (any factor) from `passwordEnabled` (cookie/login flow). Live-tested:
  401 without token, 200 with, status endpoint reports correctly.
- **NASA FIRMS connector tests** + synthetic CSV fixture. 3 tests cover
  CSV parsing, FIRMS time-format normalization (HHMM strings), idle-
  without-key path.
- **OpenAQ connector tests** + synthetic v3 measurements fixture. 4
  tests cover parsing, the X-API-Key header forwarding, dedupe via
  checkpoint, idle-without-key.
- **Sparkline component** (pure inline SVG, no Recharts). Each running
  instance card on the Connectors page now shows a 60-second-bucket
  sparkline of the past hour's record rate. 4 component tests.
- **Dashboard chunk slimmed massively.** 379 KB → 14.7 KB. Recharts
  moved into its own lazy `DashboardCharts` chunk (loads after the
  dashboard skeleton renders).

### Round 2 additions (after the first MORNING.md draft)

- **MQTT stream connector.** `connectors/mqtt`. Reference implementation
  for stream-mode connectors. Subscribes to one or more MQTT topic
  patterns; each message becomes a record. Auto-reconnects.
  Username/password via inline config or
  `SUNNY_SECRET_MQTT_USERNAME` / `SUNNY_SECRET_MQTT_PASSWORD`.
- **`/api/export` endpoint.** Streams matching events as CSV or
  Parquet. CSV streams directly; Parquet buffers to a tempfile (it has a
  footer, can't truly stream). Tested with magic-byte verification.
- **Storage `Exporter` interface.** Future ClickHouse / Postgres
  backends opt in; HTTP layer falls back to 500 if unsupported.
- **`TestPushIngestBypassesAuth`** regression test that proves the
  ingest path stays accessible even when the rest of the API is gated
  behind a session cookie.

### Bug + polish wins

- **Push ingest bypasses cookie auth.** Webhook callers can't carry a
  browser cookie. Each push connector enforces its own token (e.g.
  `X-Sunny-Token`). Documented in SECURITY.md, regression-tested in
  `TestPushIngestBypassesAuth`.

- **Tightened API base URL.** Frontend now uses relative URLs in dev too
  (Vite proxy in `vite.config.ts`). No more hardcoded `localhost:3000`.

- **Cleaned up `go vet` warnings.** Helper-fns in test code (`mustGet`,
  `mustPost`, `mustDo`) replace lazy `_ := http.Get` patterns.

- **Version bump.** Server reports `0.1.0` / phase `v0.1` instead of the
  per-phase tags. CLI matches.

## What I deliberately did NOT do

- **Did NOT verify Docker builds.** Docker daemon wasn't running. The
  Dockerfile is structurally sound (matches what the local pipeline does)
  but you should `docker compose up` once before tagging v1.

- **Did NOT touch community-decision items.** Logo, Discord vs
  Discussions, registry hosting URL — all still need your input.

- **Did NOT write marketing copy.** Per loop instructions: the launch
  artifacts (HN copy, blog posts, demo video script) are judgment-heavy
  and should be yours.

- **Did NOT commit.** All changes are in your working tree, ready for
  you to review and commit yourself.

## Files changed / added (high signal)

```
apps/server/cmd/sunny/main.go          — alert engine wired in (phase 5),
                                         auth manager wired in (phase 6)
apps/server/internal/alerts/           — NEW. Rule engine, persists triggered alerts.
apps/server/internal/auth/             — NEW. bcrypt + HMAC cookie sessions.
apps/server/internal/registry/         — NEW. Embeds registry.json.
apps/server/internal/httpapi/
  router.go                            — auth middleware, push routes, version bump
  alerts.go                            — NEW. /api/alerts CRUD.
  timeseries.go                        — NEW. /api/timeseries, /api/records/counts.
  connectors.go                        — registry endpoint, push handler dispatch
  router_test.go                       — NEW. 12 integration tests.
apps/server/internal/storage/
  storage.go                           — Timeseries, CountByConnector, alert tables, AckAlert
  timeseries_test.go                   — NEW
apps/server/internal/connectors/
  runtime.go                           — push-handler registration
apps/server/internal/connectors/builtins/builtins.go
                                       — webhook registered

connectors/
  webhook/                             — NEW. Generic push-mode connector.
  mqtt/                                — NEW. Stream-mode reference (paho).
  usgsearthquakes/connector_test.go    — NEW. + testdata/all_hour.json
  noaaweather/connector_test.go        — NEW. + testdata/alerts_active.json
  usgswater/connector_test.go          — NEW. + testdata/iv.json
  webhook/connector_test.go            — NEW.

packages/sdk-go/
  connector.go                         — PushHandler interface
  sdkhttp/                             — already existed; unchanged
packages/cli/cmd/sunny/main.go         — full CLI: hash-password, backup, restore

apps/web/src/
  api/sunny.ts                         — relative URLs, alert/registry endpoints
  api/types.ts                         — Alert, AlertRule, RegistryDocument types
  components/AuthGate.tsx              — NEW. Login screen.
  components/AuthGate.test.tsx         — NEW.
  components/layout/Layout.tsx         — mobile drawer
  components/layout/Sidebar.tsx        — drops mock copy, polls health/connectors
  pages/Connectors.tsx                 — registry merge, push UI, secrets, install snippets
  pages/Dashboard.tsx                  — server-side timeseries, real counts, no mocks
  pages/DataStreams.tsx                — real per-instance counts via /records/counts
  pages/Alerts.tsx                     — server-evaluated /api/alerts, ack button
  pages/LiveMap.tsx                    — real records with location, layer toggles
  hooks/useLiveStream.ts               — already existed; tests added
  hooks/useLiveStream.test.ts          — NEW.
  utils/format.test.ts                 — NEW.
  api/sunny.test.ts                    — NEW.
  test/setup.ts                        — NEW. testing-library/jest-dom.
App.tsx                                — React.lazy on every page

charts/sunny/                          — Helm chart (Deployment, Service, PVC, Secret, Ingress)
scripts/install.sh                     — install script with OS/arch detection
.github/workflows/ci.yml               — CI: tests + bundle smoke
.github/workflows/release.yml          — release: cross-compile + Docker + GH release
.github/ISSUE_TEMPLATE/                — bug + feature templates
.github/PULL_REQUEST_TEMPLATE.md       — PR checklist

CHANGELOG.md                           — NEW. v0.1.0 entry.
SECURITY.md                            — NEW.
CONTRIBUTING.md                        — NEW.
docs/connectors/tutorial.md            — NEW. 15-minute walkthrough.
docs/faq.md                            — NEW.
docs/semver.md                         — NEW.
docs/connector-registry-schema.json    — NEW.
docs/registry.example.json             — NEW.
README.md                              — Rewritten for v1 readiness.
```

## Quick checks before you commit

```sh
# Backend
cd apps/server && go test ./... && go vet ./...

# Connectors
cd ../../connectors && go test ./...

# Frontend
cd ../apps/web && pnpm test && pnpm build && pnpm exec tsc --noEmit -p tsconfig.app.json

# Integration
./bin/sunny &
sleep 5
curl http://localhost:3000/api/health
curl http://localhost:3000/api/version
curl -s http://localhost:3000/api/connectors | jq '.types | length'      # → 7
curl -s http://localhost:3000/api/connectors/registry | jq '.connectors | length'  # → 6
kill %1
```

## What needs your call before v1.0 tag

1. **License / branding** — logo (or none), color choice for the verified
   badge, etc.
2. **Community channel** — Discord vs GitHub Discussions. I assumed
   Discussions in the docs.
3. **Registry hosting URL** — `registry.sunny.dev/registry.json` is
   placeholder text in the docs. Either register the domain or change
   the URL.
4. **Docker test** — run `docker compose up` once locally to confirm the
   image actually builds.
5. **OpenAQ key smoke test** — if you have an OpenAQ key, drop it into
   `SUNNY_SECRET_OPENAQ_API_KEY` and confirm records flow.
6. **NASA FIRMS key smoke test** — same.
7. **HN-launch copy** — left for you per loop instructions.

Sleep good?
