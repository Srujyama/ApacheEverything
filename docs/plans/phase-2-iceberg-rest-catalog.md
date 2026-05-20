# Phase 2 sub-plan: Iceberg REST Catalog + governance

**Status:** ⬜ Not started · **Target release:** v1.2 · **Owner:** unclaimed
· **Depends on:** Phase 1 (storage + table format must be in place).

This expands [PLAN.md](../../PLAN.md) Phase 2 into a contributor-ready
roadmap. Each milestone is a PR.

---

## Goal restated

Ship an **Iceberg REST Catalog spec implementation** + a **Unity-Catalog-
style governance layer** (namespaces, tables, views, grants, audit). This
is what makes Sunny *interoperable* — any engine that speaks the spec
can read Sunny tables.

## Why this matters

- Iceberg REST Catalog is the lingua franca of open lakehouses. Speaking
  it natively unlocks Trino, Spark, DuckDB, Snowflake, and every future
  open-catalog tool without writing one-off adapters per engine.
- Governance (grants, audit, schema evolution) is the difference between
  a "lake" (chaotic blobstore of parquet files) and a "house" (governed,
  multi-tenant, auditable).

## Architecture

```
            ┌──────────────────────────────────────────────┐
            │              Iceberg REST Catalog            │
            │   apps/server/internal/catalog/rest/         │
            │                                              │
            │   GET    /v1/namespaces                      │
            │   POST   /v1/namespaces                      │
            │   GET    /v1/namespaces/{ns}/tables          │
            │   POST   /v1/namespaces/{ns}/tables          │
            │   GET    /v1/namespaces/{ns}/tables/{t}      │
            │   POST   /v1/namespaces/{ns}/tables/{t}      │
            │   ...                                        │
            └──────┬───────────────────────────────────────┘
                   │
                   ▼
            ┌──────────────────────────────────────────────┐
            │   catalog/service/  business logic           │
            │     namespace.go, table.go, view.go,         │
            │     grant.go, audit.go                       │
            └──────┬───────────────────────────────────────┘
                   │
                   ▼
            ┌──────────────────────────────────────────────┐
            │   catalog/store/   metadata persistence      │
            │   Postgres for HA; DuckDB for single-node    │
            └──────────────────────────────────────────────┘
```

## Milestones

### 2.1 Iceberg REST Catalog spec implementation

**Files:**
- `apps/server/internal/catalog/rest/v1.go` — handlers
- `apps/server/internal/catalog/rest/auth.go` — token validation per spec

**Reference:** https://iceberg.apache.org/spec/#rest-catalog-api (OpenAPI
in the repo). Implement the **required** endpoints first; defer
multi-table commit until 2.4.

**Exit criteria:**
- Pass the open-source REST catalog test suite
  (`apache/iceberg/open-api/`).
- A Spark cluster configured with `spark.sql.catalog.sunny.type=rest,
  spark.sql.catalog.sunny.uri=https://sunny/catalog` can create + query a
  table.

### 2.2 Namespaces (multi-tenant logical groupings)

**Files:**
- `apps/server/internal/catalog/service/namespace.go`

**Notes:**
- Hierarchy: namespaces are dot-separated paths (`a.b.c`).
- Properties: each namespace carries a map of arbitrary KVs (set by
  user, read by anyone with `USE`).

**Exit criteria:**
- Create/drop nested namespaces, list them, persist + reload after
  restart.

### 2.3 Tables + views CRUD via REST

**Files:**
- `apps/server/internal/catalog/service/table.go`
- `apps/server/internal/catalog/service/view.go`

**Exit criteria:**
- CRUD via REST round-trips with Spark's catalog client.
- Views: persist + return the SQL, support replacement.

### 2.4 Grants / RBAC

**Files:**
- `apps/server/internal/catalog/service/grant.go`

**Privileges:** `OWN`, `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`,
`USE`, `MANAGE_GRANTS`. Apply at namespace, table, view, or `*` scope.

**Exit criteria:**
- A user without `SELECT` on `db.t` gets 403 on every read path
  (REST + SQL).
- `GRANT`/`REVOKE` SQL through `/api/v1/query`.

### 2.5 Audit log

**Files:**
- `apps/server/internal/catalog/service/audit.go`

**What gets audited:**
- Every catalog mutation (table create/drop/alter, grant change,
  namespace change).
- Read events for tables marked `audit_reads=true`.
- Logged to an append-only `catalog_audit` table + (optionally) shipped
  via `SUNNY_AUDIT_SINK` (file/syslog/HTTP).

**Exit criteria:**
- Drop a column → audit row appears within 1s with `actor`, `action`,
  `target`, `before`, `after`.
- Audit stream can be tailed by the existing `sunny-cli watch`
  equivalent (`sunny-cli audit tail`).

### 2.6 Schema evolution

**Files:**
- `apps/server/internal/catalog/service/evolve.go`

**Operations:** add column (always safe), drop column (gated by
property), rename column (column ID stays), widen type (int → bigint,
float → double).

**Exit criteria:**
- Pass Iceberg's schema-evolution conformance tests.
- A table that adds a column then re-writes data still reads old
  snapshots correctly via time-travel.

### 2.7 Tags + table properties + comments

**Files:**
- Extend `table.go` and the REST surface.

**Exit criteria:**
- `ALTER TABLE t SET TBLPROPERTIES (...)` works via SQL.
- Tags listable in REST and SQL.

### 2.8 Cross-engine integration tests

**Files:**
- `tests/integration/spark/` — docker-compose with Spark 3.5
- `tests/integration/trino/` — Trino + iceberg connector
- `tests/integration/pyiceberg/` — PyIceberg + the REST URL

**Exit criteria:**
- All three matrices succeed in CI on every PR that touches `catalog/`.

### 2.9 Catalog federation

**Files:**
- `apps/server/internal/catalog/federation/external.go`

Allow Sunny to *mount* an external Iceberg REST catalog read-only at a
prefix (e.g., `external.snowflake.*`).

**Exit criteria:**
- A read-only mount of Snowflake's REST catalog appears in
  `/v1/namespaces` and tables are queryable via Sunny SQL.

### 2.10 Migration: built-in catalog ↔ Apache Polaris ↔ Nessie

**Files:**
- `packages/cli/cmd/sunny/catalog_migrate.go`

**Behavior:**
- Export the catalog tree (namespaces + tables + grants + audit) to a
  vendor-neutral JSON, import into Polaris or Nessie. Reverse direction
  too.

**Exit criteria:**
- Round-trip Sunny → Polaris → Sunny produces an identical catalog tree
  (modulo audit log timestamps).

## Definition of done

- ✅ Spec compliance: pass the OSS Iceberg REST catalog test suite.
- ✅ Spark, Trino, and PyIceberg can all CRUD Sunny tables through the
  REST surface.
- ✅ Grants enforced consistently across REST + SQL + the dashboard UI.
- ✅ Audit log has near-zero gaps (any catalog mutation must produce a
  row).
- ✅ A Spark `MERGE INTO sunny.db.t USING ...` round-trips correctly,
  with one new snapshot per merge.

## Open questions

- **Token format for REST:** the spec is loose. Start with the existing
  `SUNNY_API_TOKENS` Bearer flow. Phase 11 introduces ABAC; until then,
  one token = one principal.
- **Polaris coexistence:** do we ever want to *embed* Polaris instead of
  reimplementing? Decision in 2.10 after we feel the surface.

## Suggested implementation order

2.1 (spec endpoints with NOOP backing) → 2.2 → 2.3 → 2.6 (evolve in
the same milestone so we don't churn schema again) → 2.4 → 2.5 → 2.7
→ 2.8 (CI gates harden) → 2.9 → 2.10.
