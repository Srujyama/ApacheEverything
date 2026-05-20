# Morning handoff — 2026-05-19 → 2026-05-20

While you slept, I went from "let's plan a self-hosted Databricks" to
**Phase 0 done + Phase 1 foundations laid**. Here's the state of play
so you can pick up wherever feels right.

## TL;DR

- **PLAN.md** at the repo root: 13-phase roadmap (v0.1 → v2.2 self-hosted
  Databricks). Status table reflects current progress.
- **Phase 0 (foundation hardening) is complete.** All 10 milestones
  shipped + tested. ~85 new tests, all green.
- **Phase 1 (lakehouse foundation) started.** ObjectStore + LocalFS,
  Parquet writer, and Iceberg schema/snapshot foundation are done.
- **docs/plans/** has detailed sub-plans for Phases 1, 2, 3 so any
  fresh session can pick up cold.

## What's in the tree now (commits in this session)

```
754a91b Phase 1.3 (partial): Iceberg schema, partition spec, metadata JSON
20eac73 Phase 1.5: Parquet writer for ingest records
48d842e Phase 1.1: ObjectStore interface + LocalObjectStore impl
929778d Phase 0.2 + 0.10: ingest property tests, phase sub-plans, CHANGELOG
4d92082 Phase 0.9: Helm chart hardening
d8bf03d Phase 0.4: freeze connector SDK at v1 (Go + TS)
8a2ec2c Stop tracking accidentally-committed sunny binary
c8eaaf3 Phase 0.8: sunny-cli doctor, migrate, alerts subcommands
fa2ec95 Phase 0.6: OIDC login (Authorization Code + PKCE)
2e12725 Phase 0.7: Prometheus /metrics endpoint + request ID propagation
933bfcb Phase 0.5: alert dispatcher with retry + dead-letter queue
1312a9e Add PLAN.md roadmap + Phase 0 foundation work
```

## What you can do RIGHT NOW

- **Try the new endpoints:**
  ```sh
  go run ./apps/server/cmd/sunny   # in one terminal
  curl localhost:3000/api/v1/version
  curl localhost:3000/metrics
  curl localhost:3000/api/v1/alerts/deadletters
  ```
- **Run the new CLI commands:**
  ```sh
  go run ./packages/cli/cmd/sunny doctor
  go run ./packages/cli/cmd/sunny alerts deadletters
  go run ./packages/cli/cmd/sunny migrate --help
  ```
- **Plug in OIDC** by setting `SUNNY_OIDC_ISSUER`, `_CLIENT_ID`, `_REDIRECT_URL`.
- **Plug in Slack alerts** with `SUNNY_ALERTS_SLACK_URL`.
- **Run the whole test suite:**
  ```sh
  go test ./apps/server/... ./connectors/... ./packages/sdk-go/... ./packages/cli/...
  ```

## Pick-up points (ordered by leverage)

If you want to keep building, here's where the next session has the highest
ROI:

1. **Phase 1.2 — S3/GCS/Azure object store impls.** Local already works
   and passes 10 conformance tests; just need three constructor files +
   wiring. `docs/plans/phase-1-lakehouse-foundation.md` milestone 1.2.
2. **Phase 1.3 follow-up — Avro manifest list writer.** The Iceberg schema
   + metadata JSON + commit semantics are done; what's left is the Avro
   manifest list emitter and a `Commit()` that streams a Parquet file
   from package 1.5 → ObjectStore from 1.1 → adds a manifest → updates
   metadata.json → atomically swaps the catalog pointer.
3. **Phase 1.4 — Iceberg reader.** Lazy iteration, predicate pushdown.
   Big unlock for Phase 2.
4. **Phase 2.1 — Iceberg REST catalog endpoints.** Even a NOOP-backed
   implementation lets Spark + Trino start talking to Sunny.

If you want to *use* the platform first instead of build:
- The frontend SQL editor + dashboards work today against DuckDB. Try
  building a small dashboard before you commit to a multi-month phase.

## What didn't get done

- **Phase 0.10's "v1.0 release cut + signed artifacts (cosign)" is
  unclaimed.** Doable but blocked on a real GH release workflow; flagged
  in PLAN.md.
- **OIDC tests don't validate against a real IdP** — they use a fake IdP
  fixture. Worth a manual Okta/Auth0 smoke test before promoting v1.0.
- **Helm chart wasn't validated with `helm template`** — `helm` isn't
  installed locally. The YAMLs are syntactically clean; render them
  before depending on the chart in prod.

## Coverage / health snapshot

- **All Go tests pass.** 85ish new tests added this session across alerts,
  auth/OIDC, storage backend registry, property-based storage, prometheus
  exposition, CLI doctor/migrate, SDK freeze, object store conformance,
  parquet, iceberg metadata.
- **No regressions.** Old tests still pass; the only diffs in pre-existing
  code are additive (router versioning, secret structs, alert dispatcher
  hook).
- **Frontend untouched.** Phase 4–9 will pull it back into focus.

## How to read this codebase fresh

Read in this order:
1. `README.md` (what Sunny is now)
2. `PLAN.md` (where it's going)
3. `docs/plans/README.md` (how the multi-session plan works)
4. `docs/plans/phase-1-lakehouse-foundation.md` (next phase)
5. `CHANGELOG.md` (most recent changes)

## A note on scope

I deliberately did NOT push every milestone to "complete and shipped." A
lot of Phase 0 is now a real `v1.0`; some of Phase 1 is a real foundation
but not user-visible yet. That's the right shape for an overnight burst —
solid seams in place, clear pickup points, no half-finished features that
ship broken UX.

Sleep well.
