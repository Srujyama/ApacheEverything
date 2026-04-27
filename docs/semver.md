# Versioning

Sunny follows [semantic versioning](https://semver.org/) starting from
**v1.0.0**. Pre-1.0 (which is where we are now), expect breaking changes
between minor releases — pin a tag and read the release notes before
upgrading.

## What's stable

These are the contracts we commit to keeping backwards-compatible across
any minor release **after** v1.0.

### HTTP API surface

- `GET /api/health`
- `GET /api/version`
- `GET /api/connectors` (response shape: `{types, instances}`)
- `GET /api/connectors/{id}`
- `GET /api/connectors/registry`
- `GET /api/records` and its query parameters
- `GET /api/records/recent` (alias)
- `GET /api/records/counts`
- `GET /api/timeseries`
- `GET /api/alerts`
- `POST /api/alerts/{id}/ack`
- `GET/POST /api/alerts/rules`, `DELETE /api/alerts/rules/{id}`
- `GET /api/auth/status`, `POST /api/auth/login`, `POST /api/auth/logout`
- `WS /api/stream` and its query parameters
- `POST /api/ingest/{id}/...` for push connectors

We may **add** fields to JSON responses; we won't remove or rename them
without a major version bump.

### Connector SDK (Go)

The interfaces in `packages/sdk-go/connector.go` are stable:

- `Connector`, `Manifest`, `Record`, `GeoPoint`
- `Context`, `Logger`
- `PushHandler`
- `Mode`, `Category` constants

Adding fields to `Record` or methods to `Context` is allowed (additive).
Renaming or removing is a major-version change.

### Configuration

`sunny.config.yaml` keys at the top level (`addr`, `dataDir`, `connectors`)
and the `ConnectorConfig` shape (`id`, `type`, `config`) are stable. New
top-level keys may be added.

Connector-level `config` blocks are governed by each connector's own
`Manifest.ConfigSchema` — those evolve per the connector's own versioning.

### Storage layout

- The DuckDB file location (`<dataDir>/sunny.duckdb`) is stable.
- The `events`, `alerts`, `alert_rules`, `checkpoints` table names and
  their primary columns are stable. We may add columns; we won't remove them.
- Schema migrations are forward-only and idempotent. You can always upgrade
  by replacing the binary; downgrade is not supported across migrations.

### CLI commands

`sunny-cli version`, `hash-password`, `backup`, `restore` flags and exit
codes are stable.

## What's not (yet)

- Internal Go packages (`apps/server/internal/...`). These are internal
  for a reason — we'll refactor them freely.
- The TS SDK in `packages/sdk-ts/` — phase 7 sketch, not yet stable.
- The shape of the connector registry document (`docs/registry-schema.json`).
  Format changes are expected as the registry feature grows.
- Behavior of the alert rule engine on edge cases (we may tighten dedupe).
- HTML class names and CSS variables in the frontend — style how you like
  but don't depend on them in custom themes yet.

## Release cadence

No fixed cadence. Patch releases land as soon as a fix is ready. Minor
releases ship when a meaningful set of features is reviewed and tested.
Major releases are planned and documented well in advance.
