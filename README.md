# Gateway

Schema-first HTTP gateway for `agynio/api` protobuf services.

See [AGENTS.md](AGENTS.md) for source owners and contribution rules, and
[docs/catalog.json](docs/catalog.json) for operational and historical documents.

The `sync/2026-09-24-resource-lifecycle` branch rebases the forwarding acceptance
stack onto upstream `0ee317b`. Generate its clients from `spk-ai/api` `c21440b`
on `sync/2026-09-24-volume-adoption`, including imports and the internal identity
and Ziti APIs. The earlier dependency revisions below are historical records;
the published BSR module still lacks these lifecycle proposals.

Architecture: [Gateway](https://github.com/agynio/architecture/blob/main/architecture/gateway.md)

## Workload Removal Compatibility

See `RunnersGateway` in [runners.go](internal/gateway/runners.go) for the forwarding
contract. Coordinate matching generated API types across Gateway, Runners and
orchestrator.

Until the lifecycle proposals are published to BSR, generate from the sibling
API checkout at the rebased dependency above. Include the internal services
needed for a clean build; adjust sibling checkout names as needed:

```sh
cd ../api
buf generate . --template ../gateway/buf.gen.yaml --output ../gateway \
  --include-imports --path proto/agynio/api/gateway/v1 \
  --path proto/agynio/api/ziti_management/v1 --path proto/agynio/api/identity/v1
cd ../gateway
go test -race ./...
go build ./cmd/gateway
```

The [removal fixture](internal/gateway/workload_removal_test.go) uses synthetic
backend records and resolved identity. It does not replace deployed database,
Kubernetes deletion or authentication-boundary acceptance. The original
[API proposal](https://github.com/spk-ai/api/tree/feat/workload-removal-confirmation)
is historical context, not the combined build dependency.

## Local Development

### Resource Lifecycle Wire Compatibility

The following dependencies, reproduction command and results record historical
acceptance, not verification of the rebased stack.

The dependent `test/resource-lifecycle-forwarding` branch also covers native
resource anchors, preparation revocation and anchored workspace retirement.
Generate against API `23d3073` from `feat/preparation-revocation`; the public
BSR schema does not yet include these proposals. A fresh checkout needs the
internal identity/Ziti services as well as the public Gateway definitions:

```sh
cd ../api-preparation-revocation
buf generate . --template ../gateway-resource-lifecycle/buf.gen.yaml \
  --output ../gateway-resource-lifecycle --include-imports \
  --path proto/agynio/api/gateway/v1 \
  --path proto/agynio/api/ziti_management/v1 \
  --path proto/agynio/api/identity/v1
cd ../gateway-resource-lifecycle
go test -race ./...
go vet ./...
go build ./...
```

On 2026-09-15 all 363 race test entries pass with no failures or skips; build
and unfiltered vet pass. New tests cover both owner kinds over every applicable
Get/List route, exact resource and workspace identities, original reservations,
separate revocation proof/confirmation, mixed found/absent workspaces and a
zero-volume case. JSON revisions above JavaScript's exact integer range remain
decimal strings. Optional evidence stays absent until supplied by the backend.
Request filters, pagination and one downstream caller identity are preserved.

The shared real gRPC/Connect fixture uses synthetic backend records and resolved
authentication. This is wire compatibility, not independent native cleanup,
database validation, deployed authorization or A2A acceptance. No production
forwarding handler or permission changes are needed; matching generated types
are required when building an image. Generated files remain uncommitted.

The first new fixture failed to compile due to a status-enum typo. The next
whole-repository run exposed missing generated internal services; neither run
counts as acceptance. The corrected clean-generation command above includes
both services. Existing repository licensing is unchanged.

Full setup: [Local Development](https://github.com/agynio/architecture/blob/main/architecture/operations/local-development.md)

### Prepare environment

Use the Go toolchain required by [go.mod](go.mod), Buf and DevSpace. Follow the
linked bootstrap guide with an explicitly selected development cluster and
permission to change platform deployments; these commands change cluster state.

```bash
git clone https://github.com/agynio/bootstrap.git
cd bootstrap
chmod +x apply.sh
./apply.sh -y
```

See [bootstrap](https://github.com/agynio/bootstrap) for details.

### Run from sources

The executable workflow is [devspace.yaml](devspace.yaml), with
[startup](scripts/devspace-startup.sh) and [ArgoCD restoration](scripts/argocd-restore.sh)
scripts. Its published-schema generation is not a substitute for the matching
local API required by this contribution stack.

```bash
# Deploy once (exit when healthy)
devspace dev

# Watch mode (streams logs, re-syncs on changes)
devspace dev -w
```

## Adding a New API Domain

Define public domains in `agynio/api` protobuf and coordinate schema publication
with Gateway builds; do not introduce Gateway-only request/response contracts.
Keep [generation configuration](buf.gen.yaml) and [CI inputs](.github/workflows/ci.yml)
aligned with that schema. Use [existing handlers](internal/gateway/) and
[registration](cmd/gateway/main.go) for implementation patterns.
