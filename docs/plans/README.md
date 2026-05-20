# Sunny phase plans

This directory hosts **detailed sub-plans** for each phase enumerated in
the root [PLAN.md](../../PLAN.md). The root plan is the table of contents
and policy document; the files here are the actual contributor briefs.

## How to use these documents

1. **Pick a phase.** Look at PLAN.md's status table; find an unstarted
   phase or a phase with unclaimed milestones.
2. **Read the sub-plan in full** before you start writing code. They
   are self-contained: anyone (human or AI) should be able to read the
   sub-plan + PLAN.md and start shipping.
3. **Claim a milestone.** Replace `unclaimed` in PLAN.md with your
   handle + date. Open a draft PR within 3 days or release the claim.
4. **When you finish:** check off the milestone, update the status
   emoji to 🟢, link the PR. Update the sub-plan if reality diverged
   from the plan — don't leave the docs out of date.

## What's here

| File | Status | Description |
|------|--------|-------------|
| [phase-1-lakehouse-foundation.md](./phase-1-lakehouse-foundation.md) | ⬜ | Object storage + Iceberg/Delta table formats |
| [phase-2-iceberg-rest-catalog.md](./phase-2-iceberg-rest-catalog.md) | ⬜ | Iceberg REST Catalog + governance (grants, audit) |
| [phase-3-spark-on-k8s.md](./phase-3-spark-on-k8s.md) | ⬜ | Spark-on-K8s compute + Spark Connect gateway |

Phases 4-12 will get sub-plans of the same shape as they approach. Don't
write a sub-plan for a phase you're not about to start — they go stale
fast.

## Conventions

- One sub-plan = one phase. Don't split or merge phases without first
  amending PLAN.md.
- Each milestone in a sub-plan should fit in ~1–5 days of solo work for
  someone familiar with the area. Bigger → break into sub-milestones.
- Exit criteria are non-negotiable. "Looks good" is not an exit
  criterion. Use numbers (latency, throughput, test pass rate).
- Open questions stay in the sub-plan until they're resolved by code.
  Don't pretend you've decided; flag the trade-off.

## For AI sessions

If you're a coding agent picking up a phase:

1. Read PLAN.md once, the sub-plan once, then re-read both before each
   milestone.
2. The sub-plan tells you what files to create, what interface they
   expose, and what counts as "done." Trust it.
3. Always look at the most recent CHANGELOG entries + `git log -20` so
   you don't redo or contradict recent work.
4. Update PLAN.md's status table and the sub-plan's status header in
   the same PR as the code. Stale plans are worse than no plans.
