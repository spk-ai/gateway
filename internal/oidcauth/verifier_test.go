package oidcauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agynio/gateway/internal/oidctestutil"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

func newVerifier(t *testing.T, provider *oidctestutil.Provider, opts ...Option) *Verifier {
	t.Helper()

	verifier, err := NewVerifier(context.Background(), provider.Issuer, provider.ClientID, opts...)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	return verifier
}

func TestVerifierVerifySubject(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider)

	claims, err := verifier.Verify(context.Background(), provider.Token)
	if err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
	if claims.Subject != provider.Subject {
		t.Fatalf("expected subject %q, got %q", provider.Subject, claims.Subject)
	}
	if verifier.UserinfoEndpoint() != provider.UserinfoEndpoint {
		t.Fatalf("expected userinfo endpoint %q, got %q", provider.UserinfoEndpoint, verifier.UserinfoEndpoint())
	}
}

func TestVerifierVerifyAcceptsResourceServerAudience(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider)

	claims := oidc.NewAccessTokenClaims(
		provider.Issuer,
		provider.Subject,
		[]string{"https://api.example.com"},
		time.Now().Add(time.Hour),
		"jwtid",
		provider.ClientID,
		time.Second,
	)
	token := provider.SignAccessToken(t, claims)

	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
}

func TestVerifierVerifyAcceptsEmptyAudience(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider)

	now := time.Now().UTC().Add(-time.Second)
	claims := &oidc.AccessTokenClaims{
		TokenClaims: oidc.TokenClaims{
			Issuer:     provider.Issuer,
			Subject:    provider.Subject,
			Audience:   oidc.Audience{},
			Expiration: oidc.FromTime(now.Add(time.Hour)),
			IssuedAt:   oidc.FromTime(now),
			NotBefore:  oidc.FromTime(now),
			ClientID:   provider.ClientID,
			JWTID:      "jwtid",
		},
	}
	token := provider.SignAccessToken(t, claims)

	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
}

func signWithAudience(t *testing.T, provider *oidctestutil.Provider, audience []string) string {
	t.Helper()

	claims := oidc.NewAccessTokenClaims(
		provider.Issuer,
		provider.Subject,
		audience,
		time.Now().Add(time.Hour),
		"jwtid",
		provider.ClientID,
		time.Second,
	)
	return provider.SignAccessToken(t, claims)
}

// With OIDC_AUDIENCE set, `aud` must contain it: one of several audiences is
// enough, a missing, empty or different audience is rejected.
func TestVerifierVerifyAudience(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider, WithAudience(" agyn-gateway "))

	tests := []struct {
		name     string
		audience []string
		wantErr  bool
	}{
		{name: "exact", audience: []string{"agyn-gateway"}},
		{name: "one of several", audience: []string{"https://kubernetes.default.svc.cluster.local", "agyn-gateway"}},
		{name: "missing", audience: nil, wantErr: true},
		{name: "empty", audience: []string{}, wantErr: true},
		{name: "wrong", audience: []string{"https://kubernetes.default.svc.cluster.local"}, wantErr: true},
		{name: "several without it", audience: []string{"agyn-runners", "k3s"}, wantErr: true},
		{name: "case differs", audience: []string{"Agyn-Gateway"}, wantErr: true},
		{name: "prefix", audience: []string{"agyn-gateway-extra"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verifier.Verify(context.Background(), signWithAudience(t, provider, tt.audience))
			if tt.wantErr {
				if !errors.Is(err, oidc.ErrAudience) {
					t.Fatalf("expected audience error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("failed to verify token: %v", err)
			}
		})
	}
}

// Without OIDC_AUDIENCE the audience is not checked (unchanged behaviour).
func TestVerifierVerifyWithoutAudienceAcceptsAnyAudience(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider, WithAudience("  "))

	if _, err := verifier.Verify(context.Background(), signWithAudience(t, provider, []string{"someone-else"})); err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
}

func TestVerifierVerifyRejectsExpiredAndUnboundedTokens(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	verifier := newVerifier(t, provider, WithAudience("agyn-gateway"))

	now := time.Now().UTC()
	claimsWithExpiry := func(expiration oidc.Time) *oidc.AccessTokenClaims {
		return &oidc.AccessTokenClaims{
			TokenClaims: oidc.TokenClaims{
				Issuer:     provider.Issuer,
				Subject:    provider.Subject,
				Audience:   oidc.Audience{"agyn-gateway"},
				Expiration: expiration,
				IssuedAt:   oidc.FromTime(now.Add(-time.Hour)),
				NotBefore:  oidc.FromTime(now.Add(-time.Hour)),
				JWTID:      "jwtid",
			},
		}
	}

	if _, err := verifier.Verify(context.Background(), provider.SignAccessToken(t, claimsWithExpiry(oidc.FromTime(now.Add(-time.Minute))))); !errors.Is(err, oidc.ErrExpired) {
		t.Fatalf("expected expired token to be rejected, got %v", err)
	}
	if _, err := verifier.Verify(context.Background(), provider.SignAccessToken(t, claimsWithExpiry(0))); err == nil || !strings.Contains(err.Error(), "expiration claim is required") {
		t.Fatalf("expected token without exp to be rejected, got %v", err)
	}
	if _, err := verifier.Verify(context.Background(), provider.SignAccessToken(t, claimsWithExpiry(oidc.FromTime(now.Add(10*time.Minute))))); err != nil {
		t.Fatalf("failed to verify unexpired token: %v", err)
	}
}

// A token for another relying party is rejected without fetching keys.
func TestVerifierAudienceCheckedBeforeKeyFetch(t *testing.T) {
	provider := oidctestutil.NewProvider(t)
	keys := &countingKeySet{}
	verifier := newVerifier(t, provider, WithAudience("agyn-gateway"))
	verifier.keySet = keys

	if _, err := verifier.Verify(context.Background(), signWithAudience(t, provider, []string{"agyn-runners"})); err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
	if keys.calls != 0 {
		t.Fatalf("expected no key lookup, got %d", keys.calls)
	}
}

type countingKeySet struct {
	calls int
}

func (k *countingKeySet) VerifySignature(context.Context, *jose.JSONWebSignature) ([]byte, error) {
	k.calls++
	return nil, errors.New("unexpected key lookup")
}
