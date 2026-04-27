# FAQ

## What is Sunny?

A single-binary, self-hosted observability platform for physical
infrastructure. You install it on your own hardware, point connectors at
real-world data sources (USGS, NOAA, your own MQTT/Modbus sensors, an
HTTP webhook from a vendor), and get a live map, alerts, and queryable
history.

Think of it as **n8n for physical-world data**: same connector-driven
philosophy, same self-hosted-first stance.

## Why self-host?

- **Own your data.** Sensor readings, internal infrastructure, anything
  with a security or compliance angle stays on your hardware.
- **No per-event pricing.** Once it's running, throughput is yours.
- **Connect anything.** First-party connectors cover public APIs; the
  webhook connector handles anything else with HTTP.
- **One binary.** No Kafka cluster, no separate query layer, no Redis.

## How is Sunny different from Grafana / Datadog / Splunk?

- **Grafana** is a dashboard over time-series databases you operate
  separately. Sunny is the database, the ingest layer, *and* the dashboard.
- **Datadog / Splunk** are SaaS-first, log-and-metric oriented. Sunny is
  self-hosted-first and centered on records (events with location, tags,
  payload) rather than logs or metrics.
- **None of those** ship 5+ first-party connectors for U.S. public data.

## What connectors are included?

| ID | Mode | What | Auth |
| --- | --- | --- | --- |
| `usgs-earthquakes` | pull | USGS GeoJSON earthquake feeds | none |
| `noaa-weather-alerts` | pull | NWS active alerts | none |
| `usgs-water` | pull | USGS NWIS gauge readings | none |
| `nasa-firms` | pull | NASA active-fire detections (VIIRS/MODIS) | free MAP_KEY |
| `openaq` | pull | OpenAQ v3 air quality | free API key |
| `webhook` | push | generic JSON webhook with header-driven tags | optional token |
| `hello` | pull | heartbeat for development | none |

## How do I add a connector?

[15-minute tutorial.](./connectors/tutorial.md) Connectors are Go packages
that implement a small `Connector` interface. The contract is in
[`packages/sdk-go/connector.go`](../packages/sdk-go/connector.go).

## What's the storage backend?

Embedded DuckDB. One file at `<dataDir>/sunny.duckdb`. Single-writer; the
server is single-instance. Backup with `sunny-cli backup`. For
horizontally-scaled deployments, plug in ClickHouse — the `Storage`
interface is built for it (lands post-v1).

## Why DuckDB instead of SQLite or Postgres?

DuckDB is column-oriented and built for analytical queries — exactly the
workload of "time-series data with JSON payload, filter by tags, aggregate
across windows." The same workload on SQLite would need full table scans
beyond a few million rows; on Postgres you'd add Timescale, which is
another moving part.

## Can I run Sunny without auth?

Yes — leave `SUNNY_PASSWORD_HASH` unset. The API is open. Embedded /
LAN-only is the use case.

## Can multiple users share one instance?

Not yet. v1 is single-password / single-tenant. RBAC and per-user sessions
are post-v1.

## What about high availability?

Sunny is single-instance. The Helm chart deploys exactly one pod with a
PVC. For high availability, run Sunny on a node with a backup schedule
and a fast restart story; the data dir is just a DuckDB file.

## Can I export data?

Yes. DuckDB exports to Parquet, CSV, or JSON natively. SSH in and run
`COPY events TO 'export.parquet' (FORMAT PARQUET)`. Online export over
HTTP is on the roadmap.

## Is the SSPL license a problem for me?

If you're using Sunny on your own infrastructure, no — the SSPL behaves
like a permissive license for self-hosters.

If you want to **resell Sunny as a managed service** to third parties,
the SSPL requires you to open-source your entire service stack (orchestration,
monitoring, billing, …). This is the same license MongoDB and Elastic use
to prevent AWS-style resale.

If you need a different license (commercial, e.g. for embedding Sunny
in a closed-source product), email us.

## What's the project status?

Pre-1.0 alpha. Backend and API are solid, connectors are real, the UI is
functional. v1.0 is a polish-and-launch milestone — see the README's
roadmap.

## How can I contribute?

Read [CONTRIBUTING.md](../CONTRIBUTING.md) and the [connector tutorial](
./connectors/tutorial.md). PRs that ship a new connector with a smoke
test are reviewed within the week.

## How do I report a security issue?

See [SECURITY.md](../SECURITY.md). Email security@sunny.dev — please
don't open a public GitHub issue.
