# Contributing to Sunny

Welcome. This guide tells you what you need to know to ship a change to
Sunny — whether that's a new connector, a UI fix, or core platform work.

## Repo layout

```
apps/
  web/           React + TypeScript frontend (Vite, react-router, recharts)
  server/        Go server: HTTP API, ingest, storage, alert engine
                 — embeds apps/web/dist via go:embed at build time

packages/
  sdk-go/        Connector SDK (Go) — the canonical plugin contract
  sdk-ts/        Connector SDK (TypeScript) — out-of-process RPC wrapper
  cli/           sunny-cli admin tool (hash-password, backup, restore)
  core/          Shared TypeScript types

connectors/
  usgsearthquakes/   USGS Earthquake Hazards Program
  noaaweather/       NWS active alerts
  usgswater/         USGS NWIS gauges
  nasafirms/         NASA FIRMS active fires (requires API key)
  openaq/            OpenAQ v3 air quality (requires API key)
                     — pattern: each connector is a Go package; registered
                       via apps/server/internal/connectors/builtins/builtins.go

charts/sunny/    Helm chart
scripts/         install.sh
docs/            longer-form docs
```

## Local development

You need: **Go 1.25+**, **Node 20+**, **pnpm 10+**, **Docker** (optional).

```sh
pnpm install                    # web + ts deps
cd apps/server && go mod download
```

Run the dev cycle in two terminals:

```sh
# terminal 1: backend
cd apps/server && go run ./cmd/sunny

# terminal 2: web with HMR
pnpm --filter @sunny/web dev    # http://localhost:5173 → proxies API
```

The frontend, in dev mode, calls the backend at `http://localhost:3000`.
In production it's served by the same Go binary on the same port.

For the full prod-like build:

```sh
pnpm --filter @sunny/web build
cp -R apps/web/dist/. apps/server/internal/web/dist/
cd apps/server && go build -o ../../bin/sunny ./cmd/sunny
./bin/sunny                     # serves on :3000
```

## Tests

```sh
cd apps/server && go test ./...
```

There are ~30 backend tests covering bus, runtime, storage, alerts, auth,
config. Frontend tests are TODO — `pnpm tsc --noEmit` is the type-check gate.

## Adding a new connector

The 15-minute path:

1. Create `connectors/<your-connector>/connector.go`
2. Implement `sdk.Connector`: `Manifest`, `Validate`, `Run`. See
   [docs/connectors/tutorial.md](./docs/connectors/tutorial.md) for a
   step-by-step.
3. Register in `apps/server/internal/connectors/builtins/builtins.go`:

   ```go
   import yourconn "github.com/sunny/sunny/connectors/yourconnector"
   func init() { connectors.Register(yourconn.New()) }
   ```

4. Add a smoke test that calls your connector's `Run` against a mock or live
   API and verifies it publishes a sane record.
5. PR it.

The connector contract is described in
[`packages/sdk-go/connector.go`](./packages/sdk-go/connector.go). Read that
first.

## Code review expectations

- **Tests required** for non-trivial changes. The bar is "if you delete
  it, a test fails."
- **No new dependencies** without a comment in the PR explaining why.
  The project values small dependency graphs.
- **Match existing style** — `gofmt`, `goimports`, the existing TypeScript
  conventions.
- **Don't break the embedded mode.** A fresh `./bin/sunny` (no config, no
  env) must still boot, run the no-auth defaults, and show real records.
- **Don't add silent fallbacks.** Errors should propagate or log, not
  swallow.

## Before submitting a PR

- `cd apps/server && go test ./... && go vet ./...`
- `pnpm --filter @sunny/web build` (must build clean)
- `pnpm --filter @sunny/web exec tsc --noEmit -p tsconfig.app.json` (no errors)
- Squash work-in-progress commits.

## Filing issues

- Use the bug template if something's broken.
- Use the feature template for new connectors / UI ideas.
- Include the output of `sunny version` (or commit SHA) and your OS / arch.

## Releasing (maintainers)

Tag a semver version on `main`. The Docker image and binaries are built by
CI. Helm chart appVersion bumps in the same PR.

## License

By contributing, you agree your contribution is licensed under
[SSPL v1](./LICENSE) — same as the project.
