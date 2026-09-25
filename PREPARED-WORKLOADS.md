<!-- SPDX-License-Identifier: AGPL-3.0-only -->
# Prepared Workload Forwarding

This guide records historical prepared-workload forwarding acceptance. Use
[README.md](README.md) for the rebased build dependency. The active contract lives
beside `RunnersGateway` in [runners.go](internal/gateway/runners.go).
Coordinate matching API generation when building the Gateway; this contribution
adds no public mutation route or authorization policy.

## Dependencies

- Base: `6d7d432`, the local removal-confirmation Gateway acceptance branch.
- API: `spk-ai/api`, `feat/prepared-workload-inspection`, `24b73ca`.
- Branch: `test/prepared-workload-forwarding`.

From the matching API checkout, generate into this Gateway checkout:

```sh
buf generate . --template /path/to/gateway/buf.gen.yaml --output /path/to/gateway --include-imports
```

Then, from the Gateway checkout:

```sh
go test -race ./... -count=1
go build ./...
go vet ./...
```

`gen/` remains ignored. Do not bundle generated files or unrelated schema
changes into the review. The published BSR schema must include the proposed
fields before an upstream build can use that schema instead of this checkout.

## Verified

All 276 test entries, including subtests, pass under the race detector, without
exclusions or skips. Build and vet pass. The new prepared test contributes 93
entries and 70 actual HTTP requests through generated Connect handlers and a
loopback gRPC connection.

The route/state matrix and JSON assertions are maintained in
[prepared_workload_test.go](internal/gateway/prepared_workload_test.go).

The backend responses and resolved HTTP identity are fixtures. This is a wire
compatibility check, not database persistence, public authentication, Ziti
authorization, agent execution or full A2A acceptance. Prepared mutation RPCs
remain on the internal registry interface, not the public Gateway.
