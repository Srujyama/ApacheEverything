# Phase 1 sub-plan: Object storage + open table formats

**Status:** ⬜ Not started · **Target release:** v1.1 · **Owner:** unclaimed

This is the detailed sub-plan for Phase 1 of [PLAN.md](../../PLAN.md). It is
self-contained: a contributor (human or AI) can read just this file and
PLAN.md and start shipping. Treat the bullet points under each milestone
as the file-level acceptance criteria.

---

## Goal restated

Move from "DuckDB on local disk" to "Apache Iceberg + Delta Lake on object
storage." This is what makes Sunny a *lakehouse*. Without it, we're not
competing with Databricks — we're a small observability tool.

## Why this phase exists

Two reasons:
1. **External engines need a table format they can read.** Once Sunny tables
   live as Iceberg manifests on S3/MinIO, Trino, Spark, DuckDB, PyIceberg,
   Snowflake's external tables, and any future engine speak the same wire
   format. That's the foundation of the lakehouse pitch.
2. **Decouple storage from compute.** Phase 3 brings up Spark clusters that
   need to read the same data the control plane writes. Object storage +
   open table formats is the only sane sharing primitive.

## Architecture before / after

**Before (v1.0):**
```
connector → bus → Writer → DuckDB file at $SUNNY_DATA_DIR/sunny.duckdb
                              │
                              └── only Sunny can read this
```

**After (v1.1):**
```
connector → bus → Writer ──┬─→ Iceberg table on S3/MinIO (primary)
                           │       │
                           │       └── readable by Trino, Spark, PyIceberg, ...
                           │
                           └─→ DuckDB (optional, for fast local queries)

         Catalog (Phase 2): tracks tables, snapshots, ACLs.
```

## Milestones (10 PRs, ~1-3 days each)

> Claim a milestone by editing `PLAN.md` to replace `unclaimed` with your
> handle. Open a draft PR within 3 days. Sub-PRs go in `docs/plans/` if you
> need to break a milestone further.

### 1.1 Object storage abstraction (`ObjectStore` interface)

**Files to create:**
- `apps/server/internal/storage/object/object.go`

**Interface:**
```go
type ObjectStore interface {
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Put(ctx context.Context, key string, body io.Reader) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) Iter[ObjectInfo]
    // Stat returns size + mtime without fetching the body — Iceberg
    // commit logic relies on cheap existence checks.
    Stat(ctx context.Context, key string) (ObjectInfo, error)
}
type ObjectInfo struct{ Key string; Size int64; ModTime time.Time }
```

**Exit criteria:**
- Interface has zero deps beyond stdlib + `io`.
- `LocalObjectStore` in the same file (backed by `os` on a directory).
- Conformance test suite (`conformance_test.go`) that exercises every
  method against a supplied store. Phase 1.2 impls plug into it.

### 1.2 S3 + MinIO + GCS + Azure Blob + local FS implementations

**Files:**
- `apps/server/internal/storage/object/s3.go`     (covers MinIO; both use AWS SDK)
- `apps/server/internal/storage/object/gcs.go`
- `apps/server/internal/storage/object/azure.go`

**Deps to add to go.mod:**
- `github.com/aws/aws-sdk-go-v2` (S3 + MinIO)
- `cloud.google.com/go/storage` (GCS)
- `github.com/Azure/azure-sdk-for-go/sdk/storage/azblob` (Azure)

**Constructor pattern:** each impl exposes `NewS3FromEnv()` / `NewGCSFromEnv()` /
etc. that read standard provider env vars (AWS_*, GOOGLE_APPLICATION_CREDENTIALS,
AZURE_STORAGE_*) so callers don't pass credentials by hand.

**Exit criteria:**
- All four pass the conformance test suite against:
  - LocalFS: `t.TempDir()`
  - S3: MinIO in Docker (CI service container)
  - GCS: fake-gcs-server
  - Azure: Azurite emulator
- Concurrent multi-writer test: 5 goroutines `Put` to the same prefix; no
  corruption.

### 1.3 Iceberg table writer (Go) — schema, partitioning, snapshots

**Files:**
- `apps/server/internal/storage/iceberg/table.go`
- `apps/server/internal/storage/iceberg/schema.go`
- `apps/server/internal/storage/iceberg/snapshot.go`
- `apps/server/internal/storage/iceberg/manifest.go`
- `apps/server/internal/storage/iceberg/partition.go`

**Reference:** Apache Iceberg Format Spec v2,
https://iceberg.apache.org/spec/ — focus on §3 (manifests), §4
(snapshots), §5 (sorting/partitioning).

**Implementation notes:**
- We write the format ourselves rather than vendoring `apache/iceberg-go`
  because (a) it's still pre-1.0 and (b) we want zero-copy from Sunny's
  `sdk.Record`. Reconsider in 1.4 once iceberg-go stabilizes.
- Snapshots = JSON metadata files; manifests = Avro files describing
  data file sets. Both go to object storage; the catalog (Phase 2)
  tracks the "current snapshot" pointer.
- Partition by `(connector_id, day(timestamp))` initially. The schema
  evolution test in 1.6 should add a third partition column and verify
  reads of old data still work.

**Exit criteria:**
- Round-trip: write 10k records, read them back via PyIceberg in a test
  Docker container, verify row count + schema match.
- Manifests pass `iceberg-cli validate` (CI step).

### 1.4 Iceberg table reader (Go) — manifests, filtering, projection

**Files:**
- `apps/server/internal/storage/iceberg/reader.go`
- `apps/server/internal/storage/iceberg/filter.go`

**Capabilities:**
- Predicate pushdown for common filters (`connector_id = X`,
  `timestamp >= T`).
- Column projection so a `SELECT connector_id, timestamp` doesn't read
  payload bytes.
- Lazy iteration: yield records via a `Iter[sdk.Record]` so memory stays
  flat on 100M-row tables.

**Exit criteria:**
- Benchmark: scan 10M records, filter on connector_id, ≤200ms on
  laptop SSD (LocalFS backend).
- Pass cross-engine tests: Spark writes, Sunny reads; PyIceberg writes,
  Sunny reads.

### 1.5 Parquet writer (Apache Arrow Go)

**Files:**
- `apps/server/internal/storage/parquet/writer.go`

**Notes:**
- Use `github.com/apache/arrow-go/v18` (already in go.mod for connectors).
- Row group size: 128 MiB target.
- Compression: zstd default; configurable.
- Stats footers: min/max per column for every row group — Iceberg
  metadata pruning depends on these.

**Exit criteria:**
- Files readable by `parquet-tools` and PyArrow.
- Stats present and accurate (validated by reading back min/max via PyArrow).

### 1.6 Delta Lake writer (Go, via delta-go)

**Files:**
- `apps/server/internal/storage/delta/table.go`
- `apps/server/internal/storage/delta/log.go`

**Notes:**
- Delta Lake support is *secondary* in Sunny — we want it for users with
  existing Delta lakes, but Iceberg is the default.
- Use `github.com/rivian/delta-go` as the starting point; vendor if
  needed for the few writer paths it doesn't cover.

**Exit criteria:**
- Pass Delta Lake protocol conformance tests for reader/writer v2.
- A Databricks instance pointed at a Sunny-written Delta table reads it
  cleanly (manual verification; tracked in
  `docs/plans/phase-1-delta-compat-evidence.md`).

### 1.7 Snapshot expiration + compaction

**Files:**
- `apps/server/internal/storage/iceberg/maintain.go`
- `apps/server/internal/storage/iceberg/compact.go`

**Capabilities:**
- `ExpireSnapshots(olderThan time.Duration)` — drop snapshot pointers +
  data files no snapshot references.
- `RewriteDataFiles(opts)` — coalesce small files into bigger row groups.

**Exit criteria:**
- After expiration, `du -s` on object storage drops by ≥80% on a write-
  heavy test workload (1M tiny commits).
- Concurrent writer + compactor leaves the table consistent (chaos test).

### 1.8 Z-order clustering

**Files:**
- `apps/server/internal/storage/iceberg/zorder.go`

**Notes:**
- Z-order interleaves bits of multiple columns for spatial-style locality.
  Crucial for our geospatial query patterns (filter by lat+lng+time).
- Apply during `RewriteDataFiles` based on a configured column list.

**Exit criteria:**
- A spatial-bounding-box query on a Z-ordered table reads ≤30% of files
  the same query reads on a hash-partitioned table.

### 1.9 Time travel API

**Files:**
- HTTP: `apps/server/internal/httpapi/timetravel.go`
- SQL: extend `query.go` to recognize `FOR VERSION AS OF` / `FOR TIMESTAMP AS OF`

**Capabilities:**
- `SELECT * FROM t FOR VERSION AS OF <snapshot_id>` and
  `FOR TIMESTAMP AS OF <iso8601>`.
- New endpoint `GET /api/v1/tables/{ns}/{name}/snapshots` lists snapshots.

**Exit criteria:**
- Insert 1000 records, take snapshot S1. Delete half. Query with
  `FOR VERSION AS OF S1` returns the original 1000.

### 1.10 Migration tool: DuckDB → Iceberg

**Files:**
- Extend `packages/cli/cmd/sunny/migrate.go`.

**Behavior:**
- `sunny-cli migrate --from duckdb:///old.duckdb --to iceberg://...`
- Streams records in batches of 10k.
- Writes one Iceberg snapshot per 1M records to keep manifests manageable.

**Exit criteria:**
- Migrating a 100M-record DuckDB to Iceberg on LocalFS completes in
  ≤30 minutes on a laptop SSD.
- Post-migration query results match pre-migration row count + per-
  connector aggregates exactly.

## Cross-cutting tasks (every milestone touches these)

- **Tests:** every public function gets at least one test. Backend impls
  share a single conformance harness so adding a backend = adding a
  constructor + a tiny constructor test.
- **Metrics:** every milestone adds at least one Prometheus metric
  (`sunny_iceberg_snapshots_total`, `sunny_object_store_bytes_total`, …)
  to `apps/server/internal/httpapi/prometheus.go`.
- **Docs:** every milestone updates `docs/concepts/` with a 1-pager
  explainer.

## Open questions for the implementer

- **iceberg-go vs roll our own:** start by rolling our own (see 1.3). If
  iceberg-go reaches 1.0 before we ship 1.4, switch.
- **Delta v Iceberg default:** Iceberg is primary. Default table format
  in `CREATE TABLE` without `USING delta` is Iceberg.
- **Object store credentials:** read from standard provider env vars; do
  not invent a Sunny-specific format. Helm chart should pass through.

## Definition of done for Phase 1

- ✅ `OpenDSN("iceberg://...")` returns a Backend that satisfies the
  existing `RecordStore`/`CheckpointStore`/`AlertStore`/`RuleStore`
  interfaces (Phase 0.3 set this seam up).
- ✅ Pass the Iceberg conformance suite at https://github.com/apache/iceberg/tree/main/api.
- ✅ A Spark cluster pointed at Sunny's catalog (Phase 2 prereq) can
  `CREATE`, `INSERT`, `SELECT`, `MERGE`, `DROP` Sunny tables.
- ✅ Benchmark: ≥1M records/sec sustained write to Iceberg on LocalFS.
- ✅ Survive concurrent writes from two Sunny instances against the same
  table without corruption (optimistic concurrency).
- ✅ All previous (Phase 0) tests still pass.

## Suggested implementation order

If you only have time for one PR per evening: 1.1 → 1.5 → 1.3 → 1.4 →
1.7 → 1.9 → 1.10 → 1.2 (all backends in one go) → 1.6 → 1.8.

If you can parallelize two contributors: contributor A does 1.1/1.2/1.5
(storage primitives), contributor B does 1.3/1.4/1.7 (Iceberg-specific
logic), then both converge on 1.9/1.10.

---

*Sub-plan author: this document is the contract. If reality forces a
deviation, update this file in the PR — don't work around it silently.*
