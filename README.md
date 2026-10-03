# Gateway

Schema-first HTTP gateway for `agynio/api` protobuf services.

See [AGENTS.md](AGENTS.md) for source owners and contribution rules, and
[docs/catalog.json](docs/catalog.json) for operational and historical documents.

The `sync/2026-09-24-resource-lifecycle` branch rebases the forwarding acceptance
stack onto upstream `0ee317b`. [buf.gen.yaml](buf.gen.yaml) pins client generation
for CI, DevSpace and the image to `spk-ai/api` `ce64da8` on `docs/living-contracts`
(a descendant of the earlier `c21440b`), including imports and the internal
identity and Ziti APIs. The earlier dependency revisions below are historical
records; the published BSR module still lacks these lifecycle proposals.

Architecture: [Gateway](https://github.com/agynio/architecture/blob/main/architecture/gateway.md)

## Workload Removal Compatibility

See `RunnersGateway` in [runners.go](internal/gateway/runners.go) for the forwarding
contract. Coordinate matching generated API types across Gateway, Runners and
orchestrator.

Until the lifecycle proposals are published to BSR, generate from the pinned
API dependency above. Buf fetches that exact revision:

```sh
buf generate --include-imports
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

## Ziti Readiness

`/readyz` reports Ziti-listener readiness only. It is unauthenticated and
returns 200 when Ziti is disabled or the gateway service has a router-confirmed
terminator, otherwise 503. The TCP Service (Connect API, OIDC and API tokens)
does not depend on Ziti, so `/readyz` must not be its Kubernetes
`readinessProbe`: a router restart would remove every replica at once. It is
not a liveness check either: the TCP server starts before enrollment, and a bind
may take up to `ZITI_BIND_TIMEOUT` (default `90s`).

Decision (2026-10-02): keep the `/readyz` path, which orchestrator E2E already
polls, rather than renaming it to `/readyz/ziti`, and use it only for Ziti
checks such as E2E waits and alerts. The chart keeps `readinessProbe` disabled.

Startup fails closed: the process exits non-zero when no terminator is
established within `ZITI_ENROLLMENT_TIMEOUT` (default `2m`). After startup the
gateway stays up when Ziti is lost. It re-enrolls once the listener has had no
terminator for `ZITI_BIND_TIMEOUT`, sampled every 5s, or once ziti-management
reports the lease gone. A round lasts at most `ZITI_ENROLLMENT_TIMEOUT`, so
after terminators are lost Ziti is regained, or a failed round is logged, within
about bind timeout + 5s + enrollment timeout. Failed rounds are retried
indefinitely with backoff capped at 2m while `/readyz` returns 503. Identities
from failed attempts are never extended and expire with their lease.

`ZITI_SERVICE_IDENTITY_LEASE_TTL` (default `5m`) must equal ziti-management's
`SERVICE_IDENTITY_LEASE_TTL`. With `ZITI_ENABLED=true`, startup rejects settings
where `ZITI_ENROLLMENT_TIMEOUT` is shorter than the bind timeout, or where the
bind timeout, `ZITI_LEASE_RENEWAL_INTERVAL` and a 30s margin do not fit within
that lease. The [manager](internal/zitimanager/manager.go) and
[configuration](internal/platform/config.go) own the details.

## OIDC Workload Identity

Decision (2026-10-03): the A2A service pod authenticates to the Gateway with its
projected ServiceAccount token (audience `agyn-gateway`) as an OIDC bearer
token, using the Kubernetes ServiceAccount issuer as the identity provider. An
unknown subject (`system:serviceaccount:<namespace>:<name>`) is provisioned as a
new user, as for any OIDC subject. The Gateway accepts a single issuer, so this
replaces any human IdP on the same deployment.

The deployment repository sets these (chart values in parentheses); the
[verifier](internal/oidcauth/verifier.go), its
[discovery client](internal/oidcauth/discovery.go) and the
[configuration](internal/platform/config.go) own the details:

| Variable | Kubernetes issuer value | Semantics |
| --- | --- | --- |
| `OIDC_ISSUER_URL` (`oidcIssuerUrl`) | `https://kubernetes.default.svc.cluster.local` | Must equal the discovery document's `issuer`. |
| `OIDC_CLIENT_ID` (`oidcClientId`) | `agyn-gateway` | Still required; not used for verification. |
| `OIDC_AUDIENCE` (`oidcAudience`) | `agyn-gateway` | Token `aud` must contain it, otherwise rejected. |
| `OIDC_CA_FILE` (`oidcCaFile`) | `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` | PEM bundle added to the system roots for discovery and JWKS requests only. |
| `OIDC_DISCOVERY_TOKEN_FILE` (`oidcDiscoveryTokenFile`) | `/var/run/secrets/kubernetes.io/serviceaccount/token` | Sent as `Authorization: Bearer` on discovery and JWKS requests to the issuer and `jwks_uri` origins only; re-read on every fetch. |
| `OIDC_PROFILE_SOURCE` (`oidcProfileSource`) | `token` | The Kubernetes issuer has no UserInfo endpoint. |

Security boundary:
- Without `OIDC_AUDIENCE` any token the issuer signs is accepted (the earlier
  behavior, kept for existing IdPs, and logged as a warning at startup). With
  the Kubernetes issuer that would admit every pod's default API token, so
  `OIDC_DISCOVERY_TOKEN_FILE` refuses to start without it.
- Every token needs `exp`; verification is offline against the JWKS, so a
  deleted pod's bound token stays valid until it expires. Keep the projected
  token's `expirationSeconds` short.
- Any principal that can create a pod or a ServiceAccount token with audience
  `agyn-gateway` in any namespace can become an Agyn user. Restrict pod
  creation and `serviceaccounts/token` accordingly.
- The discovery token is the Gateway's own API credential. It is sent only to
  https origins (scheme, host and port) named by `OIDC_ISSUER_URL` and the
  discovered `jwks_uri`, never on a redirect to another origin, and never
  logged. Startup fails if the CA or token file is unusable.
- The Kubernetes `jwks_uri` is the API server's external endpoint, so the
  Gateway pod needs egress to it as well as to the in-cluster issuer.

## Adding a New API Domain

Define public domains in `agynio/api` protobuf and coordinate schema publication
with Gateway builds; do not introduce Gateway-only request/response contracts.
Keep [generation configuration](buf.gen.yaml) and [CI inputs](.github/workflows/ci.yml)
aligned with that schema. Use [existing handlers](internal/gateway/) and
[registration](cmd/gateway/main.go) for implementation patterns.
