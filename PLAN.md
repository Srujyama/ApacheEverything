## TL;DR

We are building **Sunny** into a **self-hosted, open-source Databricks**: a unified
lakehouse platform that combines Spark-based compute, Kafka-based streaming
ingest, Iceberg/Delta storage, a Unity-Catalog-style governance layer, notebooks,
SQL warehouses, jobs/orchestration, MLflow-style ML, dashboards, and
realtime/CDC pipelines — all installable with one binary, one Helm chart, or
one Docker Compose file.

This document is the **single source of truth for the roadmap**. Every phase is
designed so that an engineer (or an AI coding agent) can pick it up cold, read
only this file plus the linked sub-docs, and start shipping.

If you are an AI agent reading this: **always re-read the "How to use this
document" section before starting work on a phase**, and always update the
"Status" tables when you finish a milestone.

---

## How to use this document

1. **Find your phase.** Phases are numbered 0–12. Each phase has: goals,
   deliverables, exit criteria, file/package layout, and milestones.
2. **Check status.** The "Phase status" table at the top of each phase tells
   you what's done, in progress, and unclaimed.
3. **Pick an unclaimed milestone.** Each milestone is sized to ~1–5 days of
   solo work. Comment your name + date next to it before starting.
4. **Read the cross-cutting concerns.** Auth, observability, testing, CI, and
   docs are *not* optional — every phase has tasks under each.
5. **Write tests first** for any new public API. We aim for >70% coverage on
   the server, >60% on the web app.
6. **Update this file** when you finish. Don't let `PLAN.md` drift from reality
   — that defeats its purpose.

**Repo conventions** (apply to every phase):

- Go 1.25+ in `apps/server/`, `connectors/`, `packages/sdk-go/`, `packages/cli/`.
- TypeScript 5+ / React 19 / Vite in `apps/web/` and `packages/sdk-ts/`.
- One PR = one milestone. Squash-merge with a Conventional Commits title.
- `pnpm` for JS, `go work` for Go modules.
- No telemetry, ever. No required SaaS dependencies. No cloud account assumed.
- License: SSPL v1. Connectors and SDKs MAY be Apache 2.0 to allow embedding.

---

## North-star architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │              Sunny Control Plane                │
                    │  (Go server, embeds React UI, exposes REST+WS)  │
                    └─────────────────────────────────────────────────┘
                                          │
       ┌────────────────┬─────────────────┼─────────────────┬────────────────┐
       ▼                ▼                 ▼                 ▼                ▼
  ┌─────────┐    ┌────────────┐   ┌──────────────┐   ┌──────────┐    ┌────────────┐
  │ Catalog │    │  Storage   │   │   Compute    │   │  Stream  │    │  Identity  │
  │  (Iceberg│    │  (S3/MinIO│   │  (Spark on   │   │  (Kafka  │    │  (OIDC +   │
  │   REST + │    │   + Delta │   │   K8s, SQL   │   │   + Flink│    │   RBAC +   │
  │  Polaris)│    │   + Iceberg│   │   warehouses)│   │   + CDC) │    │   audit)   │
  └─────────┘    └────────────┘   └──────────────┘   └──────────┘    └────────────┘
       │                │                 │                 │                │
       └────────────────┴─────────────────┴─────────────────┴────────────────┘
                                          │
                                          ▼
                    ┌─────────────────────────────────────────────────┐
                    │             User-facing surfaces                │
                    │  Notebooks · SQL editor · Jobs · MLflow · BI    │
                    │  Dashboards · Connectors · Alerts · Lineage     │
                    └─────────────────────────────────────────────────┘
```

**Design principles:**

1. **One binary, one port** is the *out-of-the-box* experience. The full
   platform scales to K8s but a laptop install must work in <60 seconds.
2. **Open table formats only.** Apache Iceberg is primary, Delta Lake is
   secondary. We never invent a proprietary format.
3. **Open catalog API.** We speak the Iceberg REST Catalog spec so any engine
   (Trino, Snowflake, DuckDB, Spark) can read our tables.
4. **Pluggable everything.** Compute engines, storage backends, catalogs, auth
   providers, and connectors are all swappable via well-defined SDKs.
5. **Streaming is first-class.** Kafka and CDC are not afterthoughts — they
   share the same table abstraction as batch.
6. **Local-first, cloud-optional.** MinIO works as well as S3. DuckDB works as
   well as Spark for small workloads. Single-node mode must always work.

---

## Phase status overview

| Phase | Theme                                  | Status     | Target  |
|-------|----------------------------------------|------------|---------|
| 0     | Foundation hardening (current Sunny)   | 🟡 v0.1    | v1.0    |
| 1     | Object storage + Iceberg/Delta        | ⬜ Not started | v1.1 |
| 2     | Iceberg REST Catalog + governance      | ⬜          | v1.2    |
| 3     | Spark-on-Kubernetes compute            | ⬜          | v1.3    |
| 4     | SQL warehouses (Trino/Spark SQL)       | ⬜          | v1.4    |
| 5     | Notebooks (Jupyter-compatible)         | ⬜          | v1.5    |
| 6     | Jobs + orchestration                   | ⬜          | v1.6    |
| 7     | Kafka + streaming pipelines            | ⬜          | v1.7    |
| 8     | MLflow-compatible ML platform          | ⬜          | v1.8    |
| 9     | Dashboards + BI                        | ⬜          | v1.9    |
| 10    | Lineage + data quality                 | ⬜          | v2.0    |
| 11    | Multi-tenant + enterprise hardening    | ⬜          | v2.1    |
| 12    | Marketplace + ecosystem                | ⬜          | v2.2    |

Legend: ⬜ not started · 🟡 in progress · 🟢 done · 🔴 blocked

---

## Phase 0 — Foundation hardening (current → v1.0)

**Goal:** Take what Sunny already is (observability platform with DuckDB +
connectors) and harden it into a stable v1.0 foundation we can build on.

**Why this phase exists:** Every later phase assumes the current server,
connector SDK, auth, alert engine, and CLI are battle-tested. Don't skip.

### Phase 0 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 0.1 Stabilize HTTP API + versioning | unclaimed | ⬜ |
| 0.2 Cover ingest pipeline with property tests | unclaimed | ⬜ |
| 0.3 DuckDB → external storage adapter interface | unclaimed | ⬜ |
| 0.4 Connector SDK v1 freeze (Go + TS) | unclaimed | ⬜ |
| 0.5 Alert engine: dedup + retry + DLQ | unclaimed | ⬜ |
| 0.6 Auth: OIDC + service tokens (in addition to single password) | unclaimed | ⬜ |
| 0.7 Observability of self (Prometheus metrics, structured logs, traces) | unclaimed | ⬜ |
| 0.8 CLI v1: hash-password, backup, restore, migrate, doctor | unclaimed | ⬜ |
| 0.9 Helm chart hardening + air-gapped install docs | unclaimed | ⬜ |
| 0.10 v1.0 release cut + signed artifacts (cosign) | unclaimed | ⬜ |

### Phase 0 deliverables

- `apps/server/internal/api/v1/` is the only public HTTP surface; all handlers
  versioned. Breaking changes require `v2/`.
- `apps/server/internal/storage/` exposes a `Store` interface; DuckDB is one
  implementation. This is the seam Phase 1 plugs into.
- `packages/sdk-go/v1/` and `packages/sdk-ts/v1/` are frozen and semver'd.
- `packages/cli/` ships as `sunny-cli` with subcommands: `hash-password`,
  `backup`, `restore`, `migrate`, `doctor`, `connector` (scaffold/test/publish).
- `charts/sunny/` supports HA mode (read replicas, external Postgres for
  metadata).
- `docs/operate/` has runbooks for: upgrade, backup, restore, scale-out,
  disaster recovery, air-gapped install.

### Phase 0 exit criteria

- ✅ 70%+ coverage on `apps/server/`, 60%+ on `apps/web/`.
- ✅ Load test: 10k records/sec sustained ingest on a 4-core VM.
- ✅ Chaos test: kill -9 the server mid-write, restart, verify no corruption.
- ✅ All public APIs documented with OpenAPI 3.1.
- ✅ Signed release artifacts (cosign) and SBOM (syft) published to GHCR.

---

## Phase 1 — Object storage + open table formats (v1.0 → v1.1)

**Goal:** Move from "DuckDB on local disk" to "Apache Iceberg + Delta Lake on
object storage." This is the lakehouse foundation.

**Why this matters:** Databricks's moat is Delta Lake on S3. Iceberg is the
open-standard equivalent. Without this, we're not a lakehouse.

### Phase 1 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 1.1 Object storage abstraction (`ObjectStore` interface) | unclaimed | ⬜ |
| 1.2 S3 + MinIO + GCS + Azure Blob + local FS implementations | unclaimed | ⬜ |
| 1.3 Iceberg table writer (Go) — schema, partitioning, snapshots | unclaimed | ⬜ |
| 1.4 Iceberg table reader (Go) — manifests, filtering, projection | unclaimed | ⬜ |
| 1.5 Parquet writer (Apache Arrow Go) | unclaimed | ⬜ |
| 1.6 Delta Lake writer (Go, via delta-go) | unclaimed | ⬜ |
| 1.7 Snapshot expiration + compaction | unclaimed | ⬜ |
| 1.8 Z-order clustering | unclaimed | ⬜ |
| 1.9 Time travel API (`SELECT * FROM t FOR VERSION AS OF n`) | unclaimed | ⬜ |
| 1.10 Migration tool: DuckDB → Iceberg | unclaimed | ⬜ |

### Phase 1 architecture

```
apps/server/internal/storage/
├── object/          # ObjectStore interface + impls
│   ├── object.go
│   ├── s3.go
│   ├── minio.go
│   ├── gcs.go
│   ├── azure.go
│   └── local.go
├── iceberg/         # Iceberg table format
│   ├── table.go
│   ├── manifest.go
│   ├── snapshot.go
│   ├── partition.go
│   ├── schema.go
│   └── catalog/     # client to our Phase 2 catalog
├── delta/           # Delta Lake support (secondary)
│   ├── table.go
│   └── log.go
├── parquet/         # Parquet read/write via Arrow
└── format/          # shared row encoders, type system
```

### Phase 1 deliverables

- Sunny can `CREATE TABLE` an Iceberg table on S3/MinIO via SQL.
- Connector ingest writes Parquet → Iceberg snapshots, not DuckDB rows.
- `sunny-cli table compact <ns>.<table>` runs file compaction.
- `sunny-cli table expire-snapshots --older-than 7d` works.
- Time-travel queries work via REST and SQL.

### Phase 1 exit criteria

- ✅ Pass the full **Iceberg compatibility test suite** (read tables written
  by PyIceberg, Spark, Trino).
- ✅ Pass the **Delta Lake protocol conformance tests** for reader/writer v2.
- ✅ Benchmark: 1M rows/sec write to Iceberg on local MinIO.
- ✅ Survive concurrent writes from two Sunny instances against the same table
  (optimistic concurrency wins, no corruption).

---

## Phase 2 — Iceberg REST Catalog + governance (v1.1 → v1.2)

**Goal:** Ship an Iceberg REST Catalog implementation + a Unity-Catalog-style
governance layer (namespaces, tables, views, grants, audit).

**Why this matters:** This is what makes external engines (Spark, Trino,
DuckDB, Snowflake) able to talk to Sunny. The Iceberg REST Catalog spec is
the lingua franca of open lakehouses.

### Phase 2 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 2.1 Iceberg REST Catalog spec implementation | unclaimed | ⬜ |
| 2.2 Namespaces (multi-tenant logical groupings) | unclaimed | ⬜ |
| 2.3 Tables + views CRUD via REST | unclaimed | ⬜ |
| 2.4 Grants / RBAC (SELECT, INSERT, OWN, etc.) | unclaimed | ⬜ |
| 2.5 Audit log (every catalog mutation, append-only) | unclaimed | ⬜ |
| 2.6 Schema evolution (add/drop/rename columns, type widening) | unclaimed | ⬜ |
| 2.7 Tags + table properties + comments | unclaimed | ⬜ |
| 2.8 Cross-engine integration tests (Spark, Trino, PyIceberg) | unclaimed | ⬜ |
| 2.9 Catalog federation (read-only mount of external catalogs) | unclaimed | ⬜ |
| 2.10 Migration: built-in catalog ↔ Apache Polaris ↔ Nessie | unclaimed | ⬜ |

### Phase 2 architecture

```
apps/server/internal/catalog/
├── rest/            # Iceberg REST API handlers
│   ├── v1.go
│   └── auth.go
├── service/         # business logic
│   ├── namespace.go
│   ├── table.go
│   ├── view.go
│   ├── grant.go
│   └── audit.go
├── store/           # metadata persistence (Postgres or DuckDB)
└── federation/      # external catalog adapters
```

### Phase 2 exit criteria

- ✅ Iceberg-spec compliance: pass the open-source REST catalog test suite.
- ✅ A Spark cluster pointed at Sunny's catalog can `CREATE`, `INSERT`,
  `SELECT`, `MERGE`, and `DROP` tables.
- ✅ Trino can query Sunny tables via the Iceberg connector configured against
  Sunny's REST endpoint.
- ✅ RBAC: a user without `SELECT` on `db.t` gets a 403, audited.

---

## Phase 3 — Spark-on-Kubernetes compute (v1.2 → v1.3)

**Goal:** Provision, run, autoscale, and tear down Spark clusters on demand.
This is the heart of "Databricks." We use **Apache Spark Operator** for K8s
and **Spark Connect** for remote execution from the control plane.

### Phase 3 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 3.1 Cluster manager interface (`ComputeBackend`) | unclaimed | ⬜ |
| 3.2 Kubernetes Spark Operator integration | unclaimed | ⬜ |
| 3.3 Spark Connect gateway (control plane → cluster RPC) | unclaimed | ⬜ |
| 3.4 Cluster pools (warm, autoscaling, spot-aware) | unclaimed | ⬜ |
| 3.5 Cluster policies (size, image, libraries, env vars) | unclaimed | ⬜ |
| 3.6 Init scripts + library installation (pip, maven, conda) | unclaimed | ⬜ |
| 3.7 Cluster logs streaming to control plane | unclaimed | ⬜ |
| 3.8 Cluster metrics (CPU, mem, GC, executor count) | unclaimed | ⬜ |
| 3.9 Local single-node Spark mode (for laptop installs) | unclaimed | ⬜ |
| 3.10 Cost estimation + budgets | unclaimed | ⬜ |

### Phase 3 architecture

```
apps/server/internal/compute/
├── backend.go       # ComputeBackend interface
├── k8s/             # Kubernetes / Spark Operator backend
│   ├── client.go
│   ├── pool.go
│   ├── policy.go
│   └── lifecycle.go
├── local/           # single-process Spark (laptop mode)
├── connect/         # Spark Connect gRPC client
└── logs/            # log/metrics ingestion from clusters

charts/sunny/
└── templates/
    ├── spark-operator.yaml
    └── rbac.yaml
```

### Phase 3 exit criteria

- ✅ Create a cluster via REST → it appears in K8s within 60s.
- ✅ Submit a Spark Connect query from the control plane → results stream back.
- ✅ Idle cluster auto-terminates after configured TTL.
- ✅ Cluster autoscaling adds/removes executors based on pending tasks.
- ✅ Single-node mode runs a real Spark `count(*)` on a Sunny Iceberg table on
  a laptop without K8s.

---

## Phase 4 — SQL warehouses (v1.3 → v1.4)

**Goal:** A "SQL warehouse" is a managed, always-on (or serverless-feeling)
SQL endpoint. Users connect with JDBC/ODBC or via the in-browser SQL editor
and query Iceberg/Delta tables. Two backends: **Trino** (interactive) and
**Spark SQL** (heavy ETL).

### Phase 4 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 4.1 Trino integration (Iceberg connector pointed at Sunny catalog) | unclaimed | ⬜ |
| 4.2 Spark SQL warehouse via Kyuubi | unclaimed | ⬜ |
| 4.3 SQL gateway (single endpoint, routes to Trino or Spark) | unclaimed | ⬜ |
| 4.4 In-browser SQL editor (Monaco + autocomplete) | unclaimed | ⬜ |
| 4.5 Query history + saved queries | unclaimed | ⬜ |
| 4.6 Result caching + query result downloads | unclaimed | ⬜ |
| 4.7 Query profiling + EXPLAIN visualizer | unclaimed | ⬜ |
| 4.8 JDBC/ODBC drivers (or bundled Trino's) | unclaimed | ⬜ |
| 4.9 Workspace permissions on warehouses | unclaimed | ⬜ |
| 4.10 Serverless warehouse mode (warm pool, auto-resize) | unclaimed | ⬜ |

### Phase 4 architecture

```
apps/server/internal/sql/
├── gateway/         # routes queries to engines
├── trino/           # Trino client + lifecycle
├── kyuubi/          # Spark SQL via Kyuubi
├── history.go
└── savedqueries.go

apps/web/src/routes/sql/
├── Editor.tsx       # Monaco-based editor
├── Results.tsx
├── History.tsx
└── Saved.tsx
```

### Phase 4 exit criteria

- ✅ Type a SQL query in the browser → results appear in <2s on a hot warehouse.
- ✅ Connect from DBeaver via JDBC and query the same data.
- ✅ Query history is searchable and per-user.
- ✅ EXPLAIN renders a query plan tree.

---

## Phase 5 — Notebooks (v1.4 → v1.5)

**Goal:** A Jupyter-compatible notebook experience attached to Spark clusters
or SQL warehouses. Cells in Python, SQL, Scala, R. Real-time collaboration
ideal but optional for v1.5.

### Phase 5 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 5.1 Notebook data model (cells, outputs, versions) | unclaimed | ⬜ |
| 5.2 Jupyter kernel protocol bridge | unclaimed | ⬜ |
| 5.3 Spark Connect kernel (Python, Scala) | unclaimed | ⬜ |
| 5.4 SQL cell type (uses warehouse) | unclaimed | ⬜ |
| 5.5 Markdown cell + rich outputs (images, HTML, plots) | unclaimed | ⬜ |
| 5.6 Notebook UI (cell editor, output rendering, kernel status) | unclaimed | ⬜ |
| 5.7 Version history + diff | unclaimed | ⬜ |
| 5.8 `.ipynb` import/export | unclaimed | ⬜ |
| 5.9 Run-all + scheduled runs (link to Phase 6 jobs) | unclaimed | ⬜ |
| 5.10 Realtime collaboration (CRDT, optional) | unclaimed | ⬜ |

### Phase 5 exit criteria

- ✅ Open a notebook, attach to a cluster, run `df = spark.read.table("ns.t")`,
  see results.
- ✅ Mix SQL and Python cells in one notebook.
- ✅ Export to `.ipynb`, open in VS Code, run there.

---

## Phase 6 — Jobs + orchestration (v1.5 → v1.6)

**Goal:** Schedule notebooks, SQL files, JARs, Python scripts. DAGs with
dependencies. Retries, alerts, SLAs.

### Phase 6 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 6.1 Job + task data model | unclaimed | ⬜ |
| 6.2 Scheduler (cron + event-driven) | unclaimed | ⬜ |
| 6.3 Task types: notebook, SQL, Spark JAR, Python script, dbt | unclaimed | ⬜ |
| 6.4 DAG executor (topological run, parallelism, fan-out) | unclaimed | ⬜ |
| 6.5 Retries, timeouts, SLAs | unclaimed | ⬜ |
| 6.6 Job runs UI (timeline, logs, lineage) | unclaimed | ⬜ |
| 6.7 Parameters + templating (Jinja-like) | unclaimed | ⬜ |
| 6.8 Notifications (email, Slack, webhook) | unclaimed | ⬜ |
| 6.9 dbt-core integration | unclaimed | ⬜ |
| 6.10 Airflow-compatibility mode (export to Airflow DAG) | unclaimed | ⬜ |

### Phase 6 architecture

```
apps/server/internal/jobs/
├── model.go         # Job, Task, Run, TaskRun
├── scheduler.go     # cron + trigger evaluation
├── executor.go      # DAG runner, parallelism, retries
├── runners/         # one per task type
│   ├── notebook.go
│   ├── sql.go
│   ├── jar.go
│   ├── python.go
│   └── dbt.go
└── notify.go
```

### Phase 6 exit criteria

- ✅ Define a 5-task DAG via UI or YAML, schedule daily at 3am, watch it run.
- ✅ Failed task retries 3× then alerts Slack.
- ✅ Backfill last 7 days with one click.

---

## Phase 7 — Kafka + streaming pipelines (v1.6 → v1.7)

**Goal:** Kafka becomes a first-class compute layer alongside Spark. Stream
into Iceberg tables continuously. Run Flink jobs or Spark Structured Streaming
jobs. CDC from Postgres/MySQL into the lakehouse.

This is where Sunny **differentiates from Databricks** — streaming is core,
not bolted on.

### Phase 7 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 7.1 Bundled Kafka (Strimzi on K8s, embedded Redpanda for laptop) | unclaimed | ⬜ |
| 7.2 Stream → Iceberg writer (exactly-once, micro-batched) | unclaimed | ⬜ |
| 7.3 Schema Registry (Avro/Protobuf/JSON Schema) | unclaimed | ⬜ |
| 7.4 Flink-on-K8s integration (alternative compute) | unclaimed | ⬜ |
| 7.5 Spark Structured Streaming jobs as a task type | unclaimed | ⬜ |
| 7.6 Debezium CDC connectors (Postgres, MySQL, MongoDB) | unclaimed | ⬜ |
| 7.7 Stream UI: topics, consumer groups, lag, throughput | unclaimed | ⬜ |
| 7.8 Streaming SQL (Flink SQL or ksqlDB style) | unclaimed | ⬜ |
| 7.9 Delta Live Tables-style declarative pipelines (YAML) | unclaimed | ⬜ |
| 7.10 Replay + dead-letter queue UI | unclaimed | ⬜ |

### Phase 7 exit criteria

- ✅ Postgres row update → Iceberg table row updated within 5s, via Debezium.
- ✅ Define a stream → stream join in SQL, run it as a job, see metrics.
- ✅ Replay last 24h from a Kafka topic into a new Iceberg table.

---

## Phase 8 — MLflow-compatible ML platform (v1.7 → v1.8)

**Goal:** Experiment tracking, model registry, model serving. Speak the
**MLflow REST API** so existing MLflow clients work unchanged.

### Phase 8 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 8.1 MLflow Tracking API compatibility | unclaimed | ⬜ |
| 8.2 Experiment + run storage (Iceberg-backed) | unclaimed | ⬜ |
| 8.3 Artifact storage (object store) | unclaimed | ⬜ |
| 8.4 Model registry with stages (staging, prod, archived) | unclaimed | ⬜ |
| 8.5 Model serving (CPU + GPU pods on K8s) | unclaimed | ⬜ |
| 8.6 Inference gateway (REST + gRPC) | unclaimed | ⬜ |
| 8.7 Feature store (online + offline) | unclaimed | ⬜ |
| 8.8 Vector index / embeddings tables (Iceberg + ANN) | unclaimed | ⬜ |
| 8.9 Notebook-to-experiment auto-tracking | unclaimed | ⬜ |
| 8.10 GPU cluster pool | unclaimed | ⬜ |

### Phase 8 exit criteria

- ✅ `mlflow.set_tracking_uri("http://sunny:3000/mlflow")` + `mlflow.log_metric`
  works from a notebook.
- ✅ Register a model, promote to prod, deploy with one click, hit the inference
  endpoint.
- ✅ Feature store online lookup <10ms p99.

---

## Phase 9 — Dashboards + BI (v1.8 → v1.9)

**Goal:** Build dashboards inside Sunny without needing Tableau/Power BI/Looker.

### Phase 9 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 9.1 Dashboard data model (panels, layout, params) | unclaimed | ⬜ |
| 9.2 Chart library (line, bar, area, table, big number, map, heatmap) | unclaimed | ⬜ |
| 9.3 Query → chart auto-config | unclaimed | ⬜ |
| 9.4 Parameters + filters (cross-panel) | unclaimed | ⬜ |
| 9.5 Scheduled email/Slack snapshots | unclaimed | ⬜ |
| 9.6 Public sharing (signed URLs, embedded) | unclaimed | ⬜ |
| 9.7 Subscriptions (alerts on dashboard panels) | unclaimed | ⬜ |
| 9.8 Theming + branding | unclaimed | ⬜ |
| 9.9 Mobile-friendly views | unclaimed | ⬜ |
| 9.10 Grafana/Superset import | unclaimed | ⬜ |

### Phase 9 exit criteria

- ✅ Build a 6-panel exec dashboard in under 5 minutes from saved queries.
- ✅ Email a PDF snapshot every Monday at 8am.
- ✅ Embed a dashboard in an external site via signed URL.

---

## Phase 10 — Lineage + data quality (v1.9 → v2.0)

**Goal:** Column-level lineage, dataset documentation, data quality rules and
monitors. Think Unity Catalog + Monte Carlo.

### Phase 10 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 10.1 Lineage collection from Spark + Trino + jobs | unclaimed | ⬜ |
| 10.2 Column-level lineage graph (OpenLineage events) | unclaimed | ⬜ |
| 10.3 Lineage UI (graph + impact analysis) | unclaimed | ⬜ |
| 10.4 Data quality rules (Great Expectations / Soda compatible) | unclaimed | ⬜ |
| 10.5 Quality monitors + anomaly detection | unclaimed | ⬜ |
| 10.6 Documentation (description, owner, tier, tags) | unclaimed | ⬜ |
| 10.7 Glossary + business terms | unclaimed | ⬜ |
| 10.8 PII detection + classification | unclaimed | ⬜ |
| 10.9 Search across tables, columns, dashboards, notebooks | unclaimed | ⬜ |
| 10.10 Discovery / popularity ranking | unclaimed | ⬜ |

### Phase 10 exit criteria

- ✅ Click a column on a dashboard → trace upstream to source table → see job
  that produced it → see freshness SLA.
- ✅ Quality rule "no nulls in `users.email`" runs after every load, alerts on
  failure.

---

## Phase 11 — Multi-tenant + enterprise hardening (v2.0 → v2.1)

**Goal:** Run Sunny as a platform serving many teams. Workspace isolation,
quotas, billing/chargeback, SSO, SCIM, audit, network policies, customer-managed
keys.

### Phase 11 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 11.1 Workspaces (logical isolation) | unclaimed | ⬜ |
| 11.2 Resource quotas (storage, compute hours, concurrent queries) | unclaimed | ⬜ |
| 11.3 Chargeback / usage reports | unclaimed | ⬜ |
| 11.4 SSO (OIDC, SAML) | unclaimed | ⬜ |
| 11.5 SCIM (user/group provisioning) | unclaimed | ⬜ |
| 11.6 ABAC policies (attribute-based access control) | unclaimed | ⬜ |
| 11.7 Customer-managed keys (KMS, Vault) | unclaimed | ⬜ |
| 11.8 Audit log shipping (Splunk, Datadog, S3) | unclaimed | ⬜ |
| 11.9 IP allowlists, private link, VPC peering docs | unclaimed | ⬜ |
| 11.10 SOC 2 / ISO 27001 evidence collection helpers | unclaimed | ⬜ |

### Phase 11 exit criteria

- ✅ Two workspaces cannot see each other's data even if they share the
  underlying cluster.
- ✅ SSO login via Okta + group sync via SCIM works end to end.
- ✅ Every catalog mutation appears in the audit log within 1s.

---

## Phase 12 — Marketplace + ecosystem (v2.1 → v2.2)

**Goal:** Connectors, dashboards, jobs, ML models, and notebooks can be
shared, versioned, and installed from a marketplace. Plus an extension SDK
for first-class third-party UIs.

### Phase 12 status

| Milestone | Owner | Status |
|-----------|-------|--------|
| 12.1 Marketplace registry (catalog of artifacts + versions) | unclaimed | ⬜ |
| 12.2 Signing + provenance (sigstore, SLSA) | unclaimed | ⬜ |
| 12.3 In-app install flow + permissions prompts | unclaimed | ⬜ |
| 12.4 Connector marketplace (extends Sunny's existing connectors) | unclaimed | ⬜ |
| 12.5 Dashboard marketplace | unclaimed | ⬜ |
| 12.6 Notebook templates marketplace | unclaimed | ⬜ |
| 12.7 Model marketplace (Hugging Face import) | unclaimed | ⬜ |
| 12.8 Extension SDK (iframe + RPC + manifest) | unclaimed | ⬜ |
| 12.9 Public marketplace site (sunny.dev/marketplace) | unclaimed | ⬜ |
| 12.10 Commercial / paid extensions story | unclaimed | ⬜ |

---

## Cross-cutting concerns (apply to every phase)

These are not phases — they are *continuous obligations*. Every PR is
expected to keep these healthy. Each phase should add at least one task per
section.

### Security
- Threat model the new surface area before writing code.
- Validate all inputs at the API boundary.
- No secrets in logs, ever. `redacted` middleware on the logger.
- Dependency scanning in CI (govulncheck, npm audit, trivy).
- Signed releases (cosign), SBOMs (syft), reproducible builds where possible.

### Observability
- Every new package exposes Prometheus metrics under a shared registry.
- Every request gets a trace ID; structured logs include it.
- OpenTelemetry traces for cross-service calls.
- A `/healthz`, `/readyz`, `/metrics` for every component.

### Testing
- Unit tests + property tests for pure logic.
- Integration tests with real dependencies in Docker (no mocks for storage,
  catalog, Kafka, Spark, Postgres).
- E2E tests via Playwright for every user-facing flow.
- Load tests in a nightly CI run.
- Chaos tests for storage and catalog (kill mid-write, partition network).

### Docs
- Every public API has OpenAPI 3.1 docs auto-generated.
- Every concept has a 1-page explainer in `docs/concepts/`.
- Every operator-facing change updates `docs/operate/`.
- A "Hello World" tutorial that works on the latest release.

### Release
- Semver. Breaking API changes require a new major.
- Every release ships: Docker image, binary tarballs (linux/macOS, amd64/arm64),
  Helm chart, install.sh.
- CHANGELOG.md is updated in the same PR as the change.
- Release notes published as a GitHub Discussion.

### Compatibility commitments
- We commit to Iceberg REST Catalog spec compatibility.
- We commit to Delta Lake protocol v2 read/write.
- We commit to MLflow Tracking API v2 compatibility.
- We commit to Jupyter kernel protocol v5 compatibility.
- We commit to OpenLineage event spec compatibility.

---

## Roles + how multiple agents/devs can work in parallel

Solo dev can't do everything at once, but with this plan, *parallel AI sessions*
or future contributors can claim non-overlapping milestones. Suggested
parallelization seams:

- **Backend platform** (Phases 1, 2, 3, 7) — storage + catalog + compute.
- **User experience** (Phases 4, 5, 6, 9) — SQL, notebooks, jobs, dashboards.
- **ML** (Phase 8) — fully isolated until it integrates with jobs + storage.
- **Governance** (Phases 10, 11) — lineage, multi-tenant. Mostly orthogonal.
- **Ecosystem** (Phase 12) — pure additive layer on top.

A second agent can pick up Phase 4 the moment Phase 1's storage interface is
stable, even if Phase 2 catalog work is unfinished — they just stub the catalog.

---

## How to claim a milestone (for human contributors)

1. Open an issue titled `[Phase X.Y] <Milestone name>`.
2. Comment in this file: replace `unclaimed` with `@yourhandle (YYYY-MM-DD)`.
3. Open a draft PR within 3 days of claiming.
4. Mark the milestone 🟡 in the status table.
5. When merged, mark 🟢 and link the PR.

## How AI agents should consume this file

- This file is the *plan*. Plans for individual milestones go in
  `docs/plans/phase-X-Y-<slug>.md` and are scoped to one PR.
- Before starting a milestone, write a sub-plan in `docs/plans/` covering:
  files touched, interfaces, tests, risks. Get it reviewed (by a human or
  another agent) before coding.
- Do not invent new phases. If a need arises that doesn't fit, propose an
  amendment to this file as its own PR.
- Always read `MEMORY.md` and the most recent `CHANGELOG.md` entries before
  starting, so you don't redo work or contradict recent decisions.

---

## Anti-goals (things we explicitly are not doing)

- **No proprietary table format.** Iceberg + Delta only.
- **No required cloud account.** Everything runs on a laptop.
- **No telemetry phone-home.** Ever.
- **No closed-source core.** SSPL keeps competitors from rehosting, but the
  source is always open.
- **No fork of Spark / Kafka / Trino / Flink.** We integrate, we don't fork.
- **No DSL invention** where SQL or Python already work.
- **No mandatory K8s.** It scales there, but single-node mode is a first-class
  citizen forever.

---

## Open questions (resolve before each respective phase)

- **Phase 1**: Iceberg-first or Delta-first as the *default* table format? Lean Iceberg.
- **Phase 2**: Build our own catalog or fork Apache Polaris? Lean build-our-own
  (smaller surface, integrated auth) but stay spec-compatible.
- **Phase 3**: Spark Operator or Kubeflow's Spark or Volcano? Lean Spark Operator.
- **Phase 4**: Trino as the default SQL engine — agreed?
- **Phase 5**: Build a notebook UI from scratch or fork JupyterLab? Lean fork +
  embed.
- **Phase 7**: Bundle Strimzi or recommend external Kafka? Lean bundle for
  self-hosted UX.
- **Phase 8**: Implement MLflow REST surface from scratch or vendor MLflow's
  server? Lean own implementation for license + integration consistency.

---

## Glossary

- **Lakehouse** — data architecture combining the cheap storage of a data lake
  (object storage + open file formats) with the ACID guarantees and SQL of a
  warehouse.
- **Iceberg** — Apache Iceberg, an open table format on top of Parquet.
- **Delta Lake** — competing open table format, originally by Databricks.
- **Unity Catalog** — Databricks's centralized governance/catalog layer.
- **Spark Connect** — gRPC protocol to run Spark queries remotely.
- **CDC** — Change Data Capture; streaming row-level changes from a database.
- **Workspace** — a tenant boundary inside the platform.
- **Warehouse** — a managed SQL endpoint backed by Trino or Spark.

---

*Last updated: 2026-05-15. This file is canonical. If something here is wrong,
fix it in a PR — don't work around it.*
