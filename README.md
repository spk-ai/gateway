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
