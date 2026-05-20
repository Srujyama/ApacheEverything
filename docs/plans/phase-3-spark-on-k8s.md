# Phase 3 sub-plan: Spark-on-Kubernetes compute

**Status:** ⬜ Not started · **Target release:** v1.3 · **Owner:** unclaimed
· **Depends on:** Phase 1 (Iceberg storage) and Phase 2 (catalog).

---

## Goal restated

Provision, run, autoscale, and tear down **Apache Spark clusters on
demand**. This is the heart of "Databricks" — without elastic Spark
compute we're not in the game. Use **Apache Spark Operator** for K8s
lifecycle and **Spark Connect** for remote execution from the control
plane.

## Why this matters

- Databricks's primary product is a managed Spark cluster. Replacing it
  requires the cluster lifecycle to be invisible from the user — they
  click "new cluster", it shows up; they leave it idle, it tears down.
- Spark Connect (Spark 3.4+) lets the control plane issue queries to a
  cluster over gRPC, so the dashboard / SQL editor / notebooks all hit a
  single endpoint regardless of where the cluster actually runs.

## Architecture

```
   Sunny control plane (single binary)
            │
            │ gRPC (Spark Connect)
            ▼
   ┌─────────────────────────────────────┐
   │      SparkApplication CRs           │
   │  (apache/spark-on-k8s-operator)     │
   └─────────────────────────────────────┘
            │
            ▼
   ┌─────────────────────────────────────┐
   │ Spark driver + N executors on K8s   │
   │ Reads Iceberg tables via Sunny's    │
   │ REST catalog (Phase 2).             │
   └─────────────────────────────────────┘
```

## Milestones

### 3.1 Cluster manager interface (`ComputeBackend`)

**Files:**
- `apps/server/internal/compute/backend.go`

**Interface:**
```go
type ComputeBackend interface {
    CreateCluster(ctx context.Context, spec ClusterSpec) (Cluster, error)
    GetCluster(ctx context.Context, id string) (Cluster, error)
    DeleteCluster(ctx context.Context, id string) error
    ListClusters(ctx context.Context) ([]Cluster, error)
    ExecQuery(ctx context.Context, clusterID, sql string) (ResultStream, error)
}
```

**Exit criteria:**
- Interface compiles with zero impl deps.
- A `LocalBackend` impl (single-process Spark) so dev/laptop mode works
  without K8s — implemented in 3.9.

### 3.2 Kubernetes Spark Operator integration

**Files:**
- `apps/server/internal/compute/k8s/client.go` — K8s client + CR types.
- `apps/server/internal/compute/k8s/lifecycle.go` — create/wait/delete.

**Deps:**
- `k8s.io/client-go`
- `github.com/kubeflow/spark-operator/api/v1beta2` (or the maintained fork).

**Exit criteria:**
- Calling `CreateCluster` produces a `SparkApplication` CR; the impl
  blocks until the driver pod reports `Running`.
- `DeleteCluster` deletes the CR and waits for pod termination.

### 3.3 Spark Connect gateway

**Files:**
- `apps/server/internal/compute/connect/client.go`
- `apps/server/internal/compute/connect/proxy.go`

**Notes:**
- Spark Connect speaks gRPC. The control plane needs to (a) discover the
  driver's gRPC port (annotated on the driver pod), (b) tunnel queries.
- For dev (laptop) mode use direct gRPC to localhost.
- For K8s, use a service-targeted gRPC connection or port-forward via
  the K8s API.

**Exit criteria:**
- A SQL submitted via the gateway streams rows back to the control
  plane within ~1s for trivial queries.

### 3.4 Cluster pools (warm, autoscaling, spot-aware)

**Files:**
- `apps/server/internal/compute/k8s/pool.go`

**Capabilities:**
- A pool keeps N "warm" SparkApplication CRs preprovisioned to cut
  cold-start latency from ~30s to <5s.
- Autoscale executors based on `pendingTasks` reported by Spark metrics.
- `spotAware`: prefer spot instances for executors, fall back to on-
  demand if spot capacity unavailable.

**Exit criteria:**
- Pool warms two clusters at startup; submitting a query attaches to one
  in <5s.
- Killing a spot-backed executor triggers Spark's existing recovery; no
  data loss visible to the user.

### 3.5 Cluster policies

**Files:**
- `apps/server/internal/compute/k8s/policy.go`

**Policy fields:** allowed node selectors, max executors, max driver
memory, allowed images, allowed library install commands.

**Exit criteria:**
- A user attempting to spin up a cluster outside policy gets a 403 with
  a useful error message.
- Admins set policies via REST `POST /api/v1/cluster-policies`.

### 3.6 Init scripts + library installation

**Files:**
- `apps/server/internal/compute/k8s/init.go`

**Library install modes:** `pip`, `maven`, `conda`. Resolved at cluster
create; pinned by hash. Installed via Spark Operator's `dependencies`
field (Maven) and an init container (Pip/Conda).

**Exit criteria:**
- A cluster with `pip install pandas==2.2` has pandas importable from a
  notebook (Phase 5 test, but stub it now).

### 3.7 Cluster logs streaming to control plane

**Files:**
- `apps/server/internal/compute/k8s/logs.go`
- `apps/server/internal/httpapi/cluster_logs.go`

**Exit criteria:**
- `GET /api/v1/clusters/{id}/logs?follow=1` streams driver + executor
  log lines as SSE.
- 10MB/sec log throughput sustained without dropping lines.

### 3.8 Cluster metrics

**Files:**
- `apps/server/internal/compute/k8s/metrics.go`

**Metrics scraped:** Spark `/metrics/prometheus` endpoint on driver and
executor pods. Re-exported under `sunny_cluster_*` so the existing
Prometheus integration sees them.

**Exit criteria:**
- `sunny_cluster_executor_count` gauge updates within 5s of an autoscale
  event.

### 3.9 Local single-node Spark mode

**Files:**
- `apps/server/internal/compute/local/backend.go`

**Notes:**
- For laptop installs and CI, spin up Spark in-process via the
  `spark-submit` binary bundled in the Sunny image. Single executor,
  no K8s.
- Same interface (`ComputeBackend`) so the rest of the system doesn't
  know it's not on K8s.

**Exit criteria:**
- `sunny` started without K8s available still answers `POST /api/v1/sql
  {"sql": "SELECT count(*) FROM sunny.db.t"}` correctly against an
  Iceberg table written in Phase 1 tests.

### 3.10 Cost estimation + budgets

**Files:**
- `apps/server/internal/compute/k8s/cost.go`

**Inputs:** per-node-type $/hr from a values.yaml-ish config; current
cluster size; elapsed time.

**Exit criteria:**
- Cluster detail page (Phase 4/9 will render this) shows estimated cost
  to date with ±10% accuracy on AWS m-series instances.

## Definition of done

- ✅ Create a cluster via REST → it appears in K8s within 60s on a
  populated cluster pool, <5s on a warm pool.
- ✅ Submit a Spark Connect query → results stream back.
- ✅ Idle cluster auto-terminates after the configured TTL.
- ✅ Autoscaling adds/removes executors based on pending-task pressure.
- ✅ Single-node mode runs a real Spark `count(*)` on a Sunny Iceberg
  table without K8s.

## Open questions

- **Kyuubi vs Spark Connect:** Spark Connect is the official path
  forward in Spark 3.5+. Use it. Reconsider only if a specific
  capability is missing.
- **Operator choice:** apache/spark-operator (community) vs kubeflow's
  fork. Lean apache once it's GA; kubeflow's fork is the proven one
  today.

## Suggested implementation order

3.1 → 3.9 (so laptop mode works end-to-end early) → 3.2 → 3.3 → 3.5 →
3.6 → 3.7 → 3.8 → 3.4 → 3.10.
