package oidcauth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/zitadel/oidc/v3/pkg/oidc"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// TestKubernetesServiceAccountIssuer runs inside a pod only (the E2E workflow
// sets the variables): it discovers the cluster's ServiceAccount issuer with
// the pod's own API token and cluster CA, as OIDC_DISCOVERY_TOKEN_FILE and
// OIDC_CA_FILE do, then verifies the pod's projected token for the given
// audience. Tokens are never logged.
func TestKubernetesServiceAccountIssuer(t *testing.T) {
	projectedFile := os.Getenv("OIDC_KUBERNETES_SMOKE_TOKEN_FILE")
	audience := os.Getenv("OIDC_KUBERNETES_SMOKE_AUDIENCE")
	if projectedFile == "" || audience == "" {
		t.Skip("set OIDC_KUBERNETES_SMOKE_TOKEN_FILE and OIDC_KUBERNETES_SMOKE_AUDIENCE inside a pod")
	}
	projected, err := readDiscoveryToken(projectedFile)
	if err != nil {
		t.Fatal(err)
	}
	var unverified oidc.TokenClaims
	if _, err := oidc.ParseToken(projected, &unverified); err != nil {
		t.Fatalf("parse projected token: %v", err)
	}
	newVerifier := func(audience string) *Verifier {
		t.Helper()
		verifier, err := NewVerifier(context.Background(), unverified.Issuer, audience,
			WithAudience(audience),
			WithCAFile(serviceAccountDir+"/ca.crt"),
			WithDiscoveryTokenFile(serviceAccountDir+"/token"),
		)
		if err != nil {
			t.Fatalf("discover %s: %v", unverified.Issuer, err)
		}
		return verifier
	}

	verifier := newVerifier(audience)
	claims, err := verifier.Verify(context.Background(), projected)
	if err != nil {
		t.Fatalf("verify projected token: %v", err)
	}
	t.Logf("issuer %s verified subject %s for audience %s", unverified.Issuer, claims.Subject, audience)

	apiToken, err := readDiscoveryToken(serviceAccountDir + "/token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(context.Background(), apiToken); !errors.Is(err, oidc.ErrAudience) {
		t.Fatalf("expected the pod's API token to be rejected for audience %s, got %v", audience, err)
	}
	if _, err := newVerifier(audience+"-other").Verify(context.Background(), projected); !errors.Is(err, oidc.ErrAudience) {
		t.Fatalf("expected the projected token to be rejected for another audience, got %v", err)
	}
}
