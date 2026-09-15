# Gateway

Schema-first HTTP gateway built on Go 1.24.10 with ConnectRPC services generated from agynio/api
protobuf definitions.

Architecture: [Gateway](https://github.com/agynio/architecture/blob/main/architecture/gateway.md)

## Workload Removal Compatibility

The proposed `Workload.removal_confirmed_at` contract requires regenerating the
Gateway as well as Runners and the orchestrator. An older Gateway's protobuf
client can receive an unknown field over gRPC but omit it when producing JSON.
No handwritten forwarding change is needed for `ListWorkloadsByAgentInstance`.

Until the [API contribution](https://github.com/spk-ai/api/tree/feat/workload-removal-confirmation)
is published to BSR, generate from the sibling API checkout:

```sh
cd ../api
buf generate . --template ../gateway/buf.gen.yaml --output ../gateway \
  --include-imports --path proto/agynio/api/gateway/v1
cd ../gateway
go test -race ./...
go build ./cmd/gateway
```

`TestWorkloadRemovalConfirmationSurvivesGRPCToGatewayJSON` exercises the actual
gRPC client and Connect HTTP handler with a fake Runners backend. It checks that
failed/stopped billing end does not become confirmation, explicit confirmation
survives JSON serialization, and instance identity, pagination and downstream
caller identity are preserved. It makes no model calls and does not replace
deployed database, Kubernetes deletion, or authentication-boundary acceptance.

The Apps, Groups, LLM and Networks test clients embed their generated interfaces,
matching the existing Agents/Runners fakes, so the current API's additional
internal RPCs do not prevent this suite from compiling. No production handler,
permission or protocol implementation changes are included here.

## Local Development

### Resource Lifecycle Wire Compatibility

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

```bash
git clone https://github.com/agynio/bootstrap.git
cd bootstrap
chmod +x apply.sh
./apply.sh -y
```

See [bootstrap](https://github.com/agynio/bootstrap) for details.

### Run from sources

```bash
# Deploy once (exit when healthy)
devspace dev

# Watch mode (streams logs, re-syncs on changes)
devspace dev -w
```

## Adding a New API Domain

Every API domain in the gateway must be defined in protobuf and exposed via
ConnectRPC. The standard flow mirrors the existing gateway handlers:

1. Add or update the protobuf definition in `agynio/api` and include the path
   in `buf.gen.yaml` and CI `buf generate` commands.
2. Regenerate stubs with `buf generate` and implement forwarding handlers in
   `internal/gateway/<domain>.go` that satisfy the generated Connect handler
   interfaces.
3. Wire the new handler in `cmd/gateway/main.go` using the Connect mux and
   interceptors.
