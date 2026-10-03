package oidcauth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	httphelper "github.com/zitadel/oidc/v3/pkg/http"
)

const (
	// discoveryTimeout matches zitadel's httphelper.DefaultHTTPClient, which
	// is used unchanged when neither a CA file nor a token file is configured.
	discoveryTimeout = 30 * time.Second
	// maxDiscoveryTokenBytes matches the projected caller token limit in
	// internal/grpcclient; ServiceAccount tokens are a few KiB.
	maxDiscoveryTokenBytes = 16 * 1024
)

// newDiscoveryClient returns the HTTP client used for OIDC discovery and every
// JWKS fetch, including refreshes after key rotation. With no CA file and no
// token file it is zitadel's default client, so existing deployments behave
// exactly as before.
//
// The CA bundle is added to a copy of the system roots in this client's own
// transport only; http.DefaultTransport and every other client keep the
// system roots. The returned bearerTransport is nil unless a token file is
// configured; the caller must allow the discovered jwks_uri on it.
func newDiscoveryClient(issuer string, opts options) (*http.Client, *bearerTransport, error) {
	if opts.caFile == "" && opts.discoveryTokenFile == "" {
		return httphelper.DefaultHTTPClient, nil, nil
	}

	base := cloneDefaultTransport()
	if opts.caFile != "" {
		roots, err := loadRootsWithCAFile(opts.caFile)
		if err != nil {
			return nil, nil, err
		}
		base.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}

	if opts.discoveryTokenFile == "" {
		return &http.Client{Transport: base, Timeout: discoveryTimeout}, nil, nil
	}

	// Fail at startup rather than send anonymous discovery requests.
	if _, err := readDiscoveryToken(opts.discoveryTokenFile); err != nil {
		return nil, nil, err
	}
	bearer := &bearerTransport{base: base, tokenFile: opts.discoveryTokenFile, origins: map[string]struct{}{}}
	if err := bearer.allow(issuer); err != nil {
		return nil, nil, fmt.Errorf("issuer: %w", err)
	}
	return &http.Client{Transport: bearer, Timeout: discoveryTimeout}, bearer, nil
}

func cloneDefaultTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}
}

// loadRootsWithCAFile returns the system roots plus every certificate in the
// PEM bundle at path. An unreadable file, or one without a certificate, is a
// configuration error rather than a silent fallback to the system roots.
func loadRootsWithCAFile(path string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OIDC CA file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("OIDC CA file %s contains no PEM certificates", path)
	}
	return roots, nil
}

// bearerTransport attaches the token file's current contents as
// `Authorization: Bearer` to requests whose origin (scheme, host and port) is
// allowed: the issuer and, once discovered, the jwks_uri. Only https origins
// can be allowed. The header is set on a clone, never on the caller's request,
// so net/http does not copy it when following a redirect; a redirect hop is
// authorized only if its own origin is allowed.
//
// The file is re-read on every authorized request because kubelet rotates
// projected tokens; discovery runs once and JWKS fetches are rare (unknown
// key ID), so caching would save nothing. A token that cannot be read fails
// the request instead of sending it anonymously. The token is never logged
// and never appears in returned errors.
type bearerTransport struct {
	base      http.RoundTripper
	tokenFile string

	mu      sync.RWMutex
	origins map[string]struct{}
}

func (t *bearerTransport) allow(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse %q: %w", rawURL, err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("%q must use https to receive the discovery token", rawURL)
	}
	origin, ok := originOf(parsed)
	if !ok {
		return fmt.Errorf("%q has no host", rawURL)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.origins[origin] = struct{}{}
	return nil
}

func (t *bearerTransport) allowed(u *url.URL) bool {
	origin, ok := originOf(u)
	if !ok {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok = t.origins[origin]
	return ok
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.allowed(req.URL) {
		return t.base.RoundTrip(req)
	}
	token, err := readDiscoveryToken(t.tokenFile)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	authorized := req.Clone(req.Context())
	authorized.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(authorized)
}

// originOf normalizes scheme and host case and the scheme's default port, so
// https://Host and https://host:443 are one origin and :6443 is another.
func originOf(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", false
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}

// readDiscoveryToken errors name the file, never its contents.
func readDiscoveryToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open OIDC discovery token file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDiscoveryTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read OIDC discovery token file %s: %w", path, err)
	}
	if len(data) > maxDiscoveryTokenBytes {
		return "", fmt.Errorf("OIDC discovery token file %s exceeds %d bytes", path, maxDiscoveryTokenBytes)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("OIDC discovery token file %s is empty", path)
	}
	return token, nil
}
