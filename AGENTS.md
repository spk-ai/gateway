# Gateway Contribution Guide

## Owners
Start with the [forwarding contract](internal/gateway/runners.go) and its linked
fixtures. The API owns field semantics; the Gateway must not infer cleanup
evidence or expose internal lifecycle mutation RPCs as a side effect of read
compatibility.

## Documentation
- Keep implementation invariants beside their handwritten Go or protobuf owner.
  Update those comments and focused tests when behavior changes; Markdown holds
  operations, cross-repository decisions, security boundaries and dated evidence.
- Start at `docs/catalog.json`; update it when a guide's path or purpose changes.
  Keep meaningful Markdown and `AGENTS.md` discoverable using repository-relative
  paths. Do not index generated code.
- When a compatible structural navigator is available, discover repositories and
  components first, then batch-inspect selected owners and their related tests.
  Otherwise use native declarations, imports, RPC types and adjacent tests;
  `git diff upstream/main...HEAD` (or the reviewed base) identifies the changes.
  Do not add a navigator dependency or machine-specific paths to this repository.
  Navigator commands are `repos [--worktrees]`,
  `scan --repo REPO`, `inspect REPO::path` and `docs --repo REPO`.
  IDs here are `api`, `runners`, `orchestrator`, `k8s-runner` and `gateway`.
  Worktrees use Git-discovered basenames, optionally selected by `--worktree`.
  At genuine cross-repo owners, optional `@see repo::extensionless/component`
  references can aid navigation; same-repo `@see` paths retain the extension.
- Preserve dated verification, failures, skips and dependency revisions as
  historical evidence; do not silently turn them into current acceptance claims.
- Do not edit generated sources or applied SQL migrations, including comments.
  Migration bytes participate in recovery/backup checks. Explain SQL behavior
  beside the owning Go caller and link the original migration; schema changes
  require a separately reviewed additive migration. Preserve licensing/notices.

## Verification
Use existing matching `gen/` bindings. Run model-free
`go test -mod=readonly -race ./internal/gateway -run 'Test(WorkloadRemovalConfirmation|PreparedWorkload|AnchoredWorkload|AnchoredVolume)'`.
The fixtures use loopback servers and synthetic authenticated identities, not
deployed authorization, Kubernetes or a provider. Verify protobuf JSON presence
and exact decimal revisions; do not edit generated clients to make tests pass.
