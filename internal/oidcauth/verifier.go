package oidcauth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

type Claims struct {
	Subject string
	// All holds every claim in the verified token payload, so callers can read
	// provider-specific profile claims without this package knowing their names.
	All map[string]any
}

type Verifier struct {
	issuer            string
	clientID          string
	audience          string
	keySet            oidc.KeySet
	userinfoEndpoint  string
	supportedSignAlgs []string
	clockSkew         time.Duration
}

// Option configures NewVerifier. An empty value leaves the setting unset.
type Option func(*options)

type options struct {
	audience           string
	caFile             string
	discoveryTokenFile string
}

// WithAudience requires every verified token's `aud` claim to contain
// audience; a token without it is rejected. Unset, any audience the issuer
// signs is accepted, so every relying party of the issuer can authenticate
// here (with the Kubernetes issuer, every ServiceAccount token in the cluster).
func WithAudience(audience string) Option {
	return func(o *options) {
		o.audience = strings.TrimSpace(audience)
	}
}

// WithCAFile trusts the PEM bundle at path, in addition to the system roots,
// for discovery and JWKS requests only.
func WithCAFile(path string) Option {
	return func(o *options) {
		o.caFile = strings.TrimSpace(path)
	}
}

// WithDiscoveryTokenFile sends the token in path as a bearer on discovery and
// JWKS requests to the issuer and jwks_uri origins only, for issuers such as
// the Kubernetes API server that refuse anonymous discovery. The issuer and
// jwks_uri must be https. See bearerTransport.
func WithDiscoveryTokenFile(path string) Option {
	return func(o *options) {
		o.discoveryTokenFile = strings.TrimSpace(path)
	}
}

func NewVerifier(ctx context.Context, issuer, clientID string, opts ...Option) (*Verifier, error) {
	trimmedIssuer := strings.TrimSpace(issuer)
	if trimmedIssuer == "" {
		return nil, fmt.Errorf("issuer is required")
	}
	trimmedClientID := strings.TrimSpace(clientID)
	if trimmedClientID == "" {
		return nil, fmt.Errorf("client id is required")
	}
	var configured options
	for _, opt := range opts {
		opt(&configured)
	}

	// The same client serves discovery and, through the remote key set, every
	// later JWKS fetch, so CA trust and the bearer token survive key rotation.
	httpClient, bearer, err := newDiscoveryClient(trimmedIssuer, configured)
	if err != nil {
		return nil, err
	}

	discovery, err := client.Discover(ctx, trimmedIssuer, httpClient)
	if err != nil {
		return nil, err
	}
	jwksURI := strings.TrimSpace(discovery.JwksURI)
	if jwksURI == "" {
		return nil, fmt.Errorf("jwks uri missing from discovery")
	}
	if bearer != nil {
		// The Kubernetes API server advertises its external endpoint here
		// (another host and port than the in-cluster issuer). Discovery came
		// from the issuer over verified TLS, so its jwks_uri origin is trusted
		// with the token too.
		if err := bearer.allow(jwksURI); err != nil {
			return nil, fmt.Errorf("jwks uri: %w", err)
		}
	}
	userinfoEndpoint := strings.TrimSpace(discovery.UserinfoEndpoint)

	// Accept the signing algorithms the IdP actually advertises. Left unset,
	// zitadel/oidc defaults to RS256 only and rejects any other algorithm with
	// "signature algorithm not supported" — e.g. Logto signs exclusively with
	// ES384, so its tokens would never verify. Fall back to RS256 if discovery
	// omits the list.
	signAlgs := discovery.IDTokenSigningAlgValuesSupported
	if len(signAlgs) == 0 {
		signAlgs = []string{"RS256"}
	}

	return &Verifier{
		issuer:            trimmedIssuer,
		clientID:          trimmedClientID,
		audience:          configured.audience,
		keySet:            rp.NewRemoteKeySet(httpClient, jwksURI),
		userinfoEndpoint:  userinfoEndpoint,
		supportedSignAlgs: signAlgs,
		clockSkew:         time.Second,
	}, nil
}

func (v *Verifier) UserinfoEndpoint() string {
	return v.userinfoEndpoint
}

type tokenClaims struct {
	oidc.TokenClaims
}

func (v *Verifier) Verify(ctx context.Context, accessToken string) (Claims, error) {
	decrypted, err := oidc.DecryptToken(accessToken)
	if err != nil {
		return Claims{}, err
	}

	var parsed tokenClaims
	payload, err := oidc.ParseToken(decrypted, &parsed)
	if err != nil {
		return Claims{}, err
	}

	if err := oidc.CheckSubject(&parsed); err != nil {
		return Claims{}, err
	}
	if err := oidc.CheckIssuer(&parsed, v.issuer); err != nil {
		return Claims{}, err
	}
	// Checked before the signature so a token for another relying party never
	// triggers a JWKS fetch. Fails closed: no `aud`, or one without the
	// configured value, is rejected.
	if v.audience != "" {
		if err := oidc.CheckAudience(&parsed, v.audience); err != nil {
			return Claims{}, err
		}
	}
	// CheckExpiration would also reject a zero expiry; this names the cause.
	if parsed.Expiration == 0 {
		return Claims{}, fmt.Errorf("expiration claim is required")
	}
	if err := oidc.CheckSignature(ctx, decrypted, payload, &parsed, v.supportedSignAlgs, v.keySet); err != nil {
		return Claims{}, err
	}
	if err := oidc.CheckExpiration(&parsed, v.clockSkew); err != nil {
		return Claims{}, err
	}

	subject := strings.TrimSpace(parsed.Subject)
	if subject == "" {
		return Claims{}, fmt.Errorf("subject claim is required")
	}

	var all map[string]any
	if err := json.Unmarshal(payload, &all); err != nil {
		return Claims{}, err
	}

	return Claims{
		Subject: subject,
		All:     all,
	}, nil
}
