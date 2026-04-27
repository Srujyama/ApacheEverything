<!--
Thanks for the PR. Please fill out the relevant sections.
-->

## Summary

<!-- 1-3 bullets on what this PR does and why. -->

## Checklist

- [ ] `cd apps/server && go test ./...` passes
- [ ] `pnpm --filter @sunny/web build` builds cleanly
- [ ] No new dependencies, or new ones are explained below
- [ ] No silent error swallowing introduced
- [ ] Embedded mode (`./bin/sunny` with no config) still boots and shows real data

## Connector additions

If this PR adds a connector:

- [ ] Connector is registered in `apps/server/internal/connectors/builtins/builtins.go`
- [ ] `Manifest()` returns a complete `configSchema` (JSON Schema)
- [ ] `Validate()` returns descriptive errors
- [ ] Records carry `Tags`, `Location` (if applicable), and stable `SourceID`
- [ ] Checkpoint key documented in package doc comment

## New dependencies

<!-- If you added a Go module or npm package, list it and explain why. -->

## Related issues

Closes #
