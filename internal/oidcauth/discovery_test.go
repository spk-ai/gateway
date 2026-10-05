package oidcauth

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agynio/gateway/internal/oidctestutil"
	jose "github.com/go-jose/go-jose/v4"
)

const (
	workloadAudience = "agyn-gateway"
	workloadSubject  = "system:serviceaccount:a2a:a2a-service"
)

// kubeIssuer mimics the Kubernetes ServiceAccount issuer: discovery is served
// by the issuer origin, the JWKS by a different origin (the API server's
// external endpoint), both with a private CA and both refusing requests
// without the currently valid bearer token.
type kubeIssuer struct {
	issuer *httptest.Server
	jwks   *httptest.Server

	mu            sync.Mutex
	validBearer   string
	keys          jose.JSONWebKeySet
	jwksURI       string
	discoveryAuth []string
	jwksAuth      []string
}

func newKubeIssuer(t *testing.T, validBearer string) *kubeIssuer {
	t.Helper()

	k := &kubeIssuer{validBearer: validBearer}
	k.issuer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		if !k.authorize(w, r, &k.discoveryAuth) {
			return
		}
		k.mu.Lock()
		jwksURI := k.jwksURI
		k.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                k.issuer.URL,
			"jwks_uri":                              jwksURI,
			"response_types_supported":              []string{"id_token"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(k.issuer.Close)
	k.jwks = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openid/v1/jwks" {
			http.NotFound(w, r)
			return
		}
		if !k.authorize(w, r, &k.jwksAuth) {
			return
		}
		k.mu.Lock()
		keys := k.keys
		k.mu.Unlock()
		_ = json.NewEncoder(w).Encode(keys)
	}))
	t.Cleanup(k.jwks.Close)
	k.jwksURI = k.jwks.URL + "/openid/v1/jwks"
	return k
}

// authorize records the Authorization header and answers 401 like the API
// server does for anonymous or stale callers.
func (k *kubeIssuer) authorize(w http.ResponseWriter, r *http.Request, seen *[]string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	header := r.Header.Get("Authorization")
	*seen = append(*seen, header)
	// An empty validBearer models an issuer that serves anonymous callers.
	want := ""
	if k.validBearer != "" {
		want = "Bearer " + k.validBearer
	}
	if header != want {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","message":"Unauthorized","code":401}`))
		return false
	}
	return true
}

func (k *kubeIssuer) setValidBearer(token string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.validBearer = token
}

func (k *kubeIssuer) setJWKSURI(uri string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.jwksURI = uri
}

func (k *kubeIssuer) addKey(key *rsa.PrivateKey, keyID string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys.Keys = append(k.keys.Keys, oidctestutil.NewJWKS(key, keyID).Keys...)
}

func (k *kubeIssuer) seen() (discovery, jwks []string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.discoveryAuth...), append([]string(nil), k.jwksAuth...)
}

// caFile writes the httptest certificate, which every TLS test server shares,
// as a PEM bundle.
func caFile(t *testing.T, server *httptest.Server) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeProjectedToken mimics kubelet's atomic writer: the token path is a
// symlink into a timestamped directory that is swapped on rotation.
func writeProjectedToken(t *testing.T, dir, token string) string {
	t.Helper()
	data, err := os.MkdirTemp(dir, "..data-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "token"), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "token")
	tmp := link + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(filepath.Join(data, "token"), tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func signWorkloadToken(t *testing.T, issuer string, key *rsa.PrivateKey, keyID string, audience []string) string {
	t.Helper()
	now := time.Now()
	return oidctestutil.SignToken(t, oidctestutil.NewSigner(t, key, keyID), map[string]any{
		"iss": issuer,
		"sub": workloadSubject,
		"aud": audience,
		"exp": now.Add(10 * time.Minute).Unix(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"kubernetes.io": map[string]any{
			"namespace":      "a2a",
			"serviceaccount": map[string]any{"name": "a2a-service", "uid": "sa-uid"},
		},
	})
}

func newWorkloadVerifier(t *testing.T, k *kubeIssuer, ca, tokenFile string) (*Verifier, error) {
	t.Helper()
	return NewVerifier(context.Background(), k.issuer.URL, workloadAudience,
		WithAudience(workloadAudience),
		WithCAFile(ca),
		WithDiscoveryTokenFile(tokenFile),
	)
}

func assertHeaders(t *testing.T, kind string, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s Authorization headers %q, want %q", kind, got, want)
	}
}

func TestVerifierWithKubernetesIssuerCAAndBearer(t *testing.T) {
	k := newKubeIssuer(t, "sa.token.one")
	key := oidctestutil.NewRSAKey(t)
	k.addKey(key, "key-1")
	tokenFile := writeProjectedToken(t, t.TempDir(), "sa.token.one\n")

	verifier, err := newWorkloadVerifier(t, k, caFile(t, k.issuer), tokenFile)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	if verifier.UserinfoEndpoint() != "" {
		t.Fatalf("unexpected userinfo endpoint %q", verifier.UserinfoEndpoint())
	}

	claims, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{workloadAudience}))
	if err != nil {
		t.Fatalf("failed to verify workload token: %v", err)
	}
	if claims.Subject != workloadSubject {
		t.Fatalf("unexpected subject %q", claims.Subject)
	}
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{"https://kubernetes.default.svc.cluster.local"})); err == nil {
		t.Fatal("expected a default API-audience token to be rejected")
	}

	discovery, jwks := k.seen()
	assertHeaders(t, "discovery", discovery, []string{"Bearer sa.token.one"})
	assertHeaders(t, "jwks", jwks, []string{"Bearer sa.token.one"})
}

// The CA is trusted by the OIDC client only: without it discovery fails, and
// adding it does not change the process-wide default transport.
func TestVerifierCATrustIsScopedToOIDC(t *testing.T) {
	k := newKubeIssuer(t, "sa.token")
	tokenFile := writeProjectedToken(t, t.TempDir(), "sa.token")

	_, err := NewVerifier(context.Background(), k.issuer.URL, workloadAudience,
		WithAudience(workloadAudience), WithDiscoveryTokenFile(tokenFile))
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("expected an untrusted certificate error without the CA, got %v", err)
	}
	if discovery, _ := k.seen(); len(discovery) != 0 {
		t.Fatalf("no request may complete without a trusted certificate, got %q", discovery)
	}

	if _, err := newWorkloadVerifier(t, k, caFile(t, k.issuer), tokenFile); err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	resp, err := http.Get(k.issuer.URL + "/.well-known/openid-configuration")
	if err == nil {
		resp.Body.Close()
		t.Fatal("the OIDC CA must not be trusted by http.DefaultTransport")
	}
}

func TestVerifierCAOnlyWithoutBearer(t *testing.T) {
	k := newKubeIssuer(t, "")
	key := oidctestutil.NewRSAKey(t)
	k.addKey(key, "key-1")

	verifier, err := NewVerifier(context.Background(), k.issuer.URL, workloadAudience,
		WithAudience(workloadAudience), WithCAFile(caFile(t, k.issuer)))
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{workloadAudience})); err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
	discovery, jwks := k.seen()
	assertHeaders(t, "discovery", discovery, []string{""})
	assertHeaders(t, "jwks", jwks, []string{""})
}

// A redirect from an allowed origin to another origin never carries the
// token, and neither does any other request on the same client.
func TestDiscoveryBearerNotSentToOtherOrigins(t *testing.T) {
	k := newKubeIssuer(t, "sa.token")
	key := oidctestutil.NewRSAKey(t)
	k.addKey(key, "key-1")

	var otherMu sync.Mutex
	var otherAuth []string
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherMu.Lock()
		otherAuth = append(otherAuth, r.Header.Get("Authorization"))
		otherMu.Unlock()
		k.mu.Lock()
		keys := k.keys
		k.mu.Unlock()
		_ = json.NewEncoder(w).Encode(keys)
	}))
	t.Cleanup(other.Close)
	redirector := httptest.NewUnstartedServer(nil)
	redirector.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sa.token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, other.URL+"/keys", http.StatusFound)
	})
	redirector.StartTLS()
	t.Cleanup(redirector.Close)
	// The jwks_uri origin is allowed, so it gets the token; its redirect
	// target is not.
	k.setJWKSURI(redirector.URL + "/openid/v1/jwks")

	tokenFile := writeProjectedToken(t, t.TempDir(), "sa.token")
	verifier, err := newWorkloadVerifier(t, k, caFile(t, k.issuer), tokenFile)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{workloadAudience})); err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}

	client, _, err := newDiscoveryClient(k.issuer.URL, options{caFile: caFile(t, k.issuer), discoveryTokenFile: tokenFile})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(other.URL + "/direct")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	otherMu.Lock()
	defer otherMu.Unlock()
	assertHeaders(t, "other origin", otherAuth, []string{"", ""})
}

// Kubelet rotates the token file and the issuer rotates its signing key: the
// next JWKS fetch, on the same client, re-reads the file and sends the new
// token, which is the only one the issuer still accepts.
func TestDiscoveryTokenAndSigningKeyRotation(t *testing.T) {
	k := newKubeIssuer(t, "sa.token.one")
	firstKey := oidctestutil.NewRSAKey(t)
	k.addKey(firstKey, "key-1")
	dir := t.TempDir()
	tokenFile := writeProjectedToken(t, dir, "sa.token.one")

	verifier, err := newWorkloadVerifier(t, k, caFile(t, k.issuer), tokenFile)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, firstKey, "key-1", []string{workloadAudience})); err != nil {
		t.Fatalf("failed to verify first token: %v", err)
	}

	writeProjectedToken(t, dir, "sa.token.two")
	k.setValidBearer("sa.token.two")
	secondKey := oidctestutil.NewRSAKey(t)
	k.addKey(secondKey, "key-2")

	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, secondKey, "key-2", []string{workloadAudience})); err != nil {
		t.Fatalf("failed to verify token signed with the rotated key: %v", err)
	}
	// Cached keys still verify without another fetch.
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, firstKey, "key-1", []string{workloadAudience})); err != nil {
		t.Fatalf("failed to verify token signed with the first key: %v", err)
	}

	discovery, jwks := k.seen()
	assertHeaders(t, "discovery", discovery, []string{"Bearer sa.token.one"})
	assertHeaders(t, "jwks", jwks, []string{"Bearer sa.token.one", "Bearer sa.token.two"})
}

// A token that becomes unreadable fails the JWKS fetch instead of sending an
// anonymous request.
func TestDiscoveryTokenUnreadableFailsClosed(t *testing.T) {
	k := newKubeIssuer(t, "sa.token")
	key := oidctestutil.NewRSAKey(t)
	k.addKey(key, "key-1")
	tokenFile := writeProjectedToken(t, t.TempDir(), "sa.token")

	verifier, err := newWorkloadVerifier(t, k, caFile(t, k.issuer), tokenFile)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	if err := os.Remove(tokenFile); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{workloadAudience})); err == nil {
		t.Fatal("expected verification to fail without a readable discovery token")
	}
	if _, jwks := k.seen(); len(jwks) != 0 {
		t.Fatalf("expected no JWKS request, got %q", jwks)
	}
}

func TestDiscoveryTokenNeverLogged(t *testing.T) {
	const secret = "sa.token.secret-value"
	var logs bytes.Buffer
	previousOutput, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	previousSlog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
		slog.SetDefault(previousSlog)
	})

	k := newKubeIssuer(t, "something-else")
	key := oidctestutil.NewRSAKey(t)
	k.addKey(key, "key-1")
	ca := caFile(t, k.issuer)
	tokenFile := writeProjectedToken(t, t.TempDir(), secret)

	var errs []error
	// Discovery refused.
	_, err := newWorkloadVerifier(t, k, ca, tokenFile)
	errs = append(errs, err)

	// JWKS refused after a successful discovery.
	k.setValidBearer(secret)
	verifier, err := newWorkloadVerifier(t, k, ca, tokenFile)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	k.setValidBearer("something-else")
	_, err = verifier.Verify(context.Background(), signWorkloadToken(t, k.issuer.URL, key, "key-1", []string{workloadAudience}))
	errs = append(errs, err)

	for _, err := range errs {
		if err == nil {
			t.Fatal("expected a refused request to fail")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error exposes the discovery token: %v", err)
		}
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("logs expose the discovery token")
	}
}

func TestNewVerifierRejectsUnusableDiscoverySettings(t *testing.T) {
	k := newKubeIssuer(t, "sa.token")
	ca := caFile(t, k.issuer)
	dir := t.TempDir()
	tokenFile := writeProjectedToken(t, dir, "sa.token")
	notPEM := filepath.Join(dir, "not-a-ca.crt")
	if err := os.WriteFile(notPEM, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	emptyToken := filepath.Join(dir, "empty-token")
	if err := os.WriteFile(emptyToken, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bigToken := filepath.Join(dir, "big-token")
	if err := os.WriteFile(bigToken, []byte(strings.Repeat("a", maxDiscoveryTokenBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := oidctestutil.NewProvider(t)

	tests := []struct {
		name    string
		issuer  string
		jwksURI string
		opts    []Option
		want    string
	}{
		{name: "missing CA file", issuer: k.issuer.URL, opts: []Option{WithCAFile(filepath.Join(dir, "missing.crt"))}, want: "OIDC CA file"},
		{name: "CA file without certificates", issuer: k.issuer.URL, opts: []Option{WithCAFile(notPEM)}, want: "no PEM certificates"},
		{name: "missing token file", issuer: k.issuer.URL, opts: []Option{WithCAFile(ca), WithDiscoveryTokenFile(filepath.Join(dir, "missing"))}, want: "discovery token file"},
		{name: "empty token file", issuer: k.issuer.URL, opts: []Option{WithCAFile(ca), WithDiscoveryTokenFile(emptyToken)}, want: "is empty"},
		{name: "oversized token file", issuer: k.issuer.URL, opts: []Option{WithCAFile(ca), WithDiscoveryTokenFile(bigToken)}, want: "exceeds"},
		{name: "plain http issuer with token", issuer: plain.Issuer, opts: []Option{WithDiscoveryTokenFile(tokenFile)}, want: "must use https"},
		{name: "plain http jwks_uri with token", issuer: k.issuer.URL, jwksURI: plain.Issuer + "/jwks", opts: []Option{WithCAFile(ca), WithDiscoveryTokenFile(tokenFile)}, want: "jwks uri"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwksURI := k.jwks.URL + "/openid/v1/jwks"
			if tt.jwksURI != "" {
				jwksURI = tt.jwksURI
			}
			k.setJWKSURI(jwksURI)
			_, err := NewVerifier(context.Background(), tt.issuer, workloadAudience, append([]Option{WithAudience(workloadAudience)}, tt.opts...)...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestOriginOf(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "https://kubernetes.default.svc.cluster.local", want: "https://kubernetes.default.svc.cluster.local:443", ok: true},
		{raw: "https://Kubernetes.Default.svc.cluster.local:443/openid", want: "https://kubernetes.default.svc.cluster.local:443", ok: true},
		{raw: "https://198.51.100.7:6443/openid/v1/jwks", want: "https://198.51.100.7:6443", ok: true},
		{raw: "HTTPS://[::1]/x", want: "https://[::1]:443", ok: true},
		{raw: "http://issuer.example.com", want: "http://issuer.example.com:80", ok: true},
		{raw: "ftp://issuer.example.com", ok: false},
		{raw: "/relative", ok: false},
	}
	for _, tt := range tests {
		parsed, err := url.Parse(tt.raw)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := originOf(parsed)
		if ok != tt.ok || got != tt.want {
			t.Errorf("originOf(%q) = %q, %v; want %q, %v", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}

func TestBearerTransportRequiresReadableTokenOnlyForAllowedOrigins(t *testing.T) {
	var gotAuth []string
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotAuth = append(gotAuth, req.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	transport := &bearerTransport{base: base, tokenFile: filepath.Join(t.TempDir(), "missing"), origins: map[string]struct{}{}}
	if err := transport.allow("https://issuer.example.com"); err != nil {
		t.Fatal(err)
	}

	request := func(raw string) error {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := transport.RoundTrip(req)
		if err == nil {
			resp.Body.Close()
		}
		if req.Header.Get("Authorization") != "" {
			t.Fatal("the caller's request must not be modified")
		}
		return err
	}
	if err := request("https://issuer.example.com/.well-known/openid-configuration"); err == nil {
		t.Fatal("expected an unreadable token to fail an allowed request")
	}
	if err := request("https://other.example.com/"); err != nil {
		t.Fatalf("unexpected error for another origin: %v", err)
	}
	if err := request("https://issuer.example.com:8443/"); err != nil {
		t.Fatalf("unexpected error for another port: %v", err)
	}
	assertHeaders(t, "base", gotAuth, []string{"", ""})
	if err := transport.allow("http://issuer.example.com"); err == nil {
		t.Fatal("expected a plain http origin to be refused")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
