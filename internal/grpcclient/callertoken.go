package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// CallerTokenMetadataKey carries this service's projected ServiceAccount token
// to Runners, which validates it with a Kubernetes TokenReview.
//
// @see runners::internal/rpcauth/interceptor
const CallerTokenMetadataKey = "x-agyn-caller-token"

const (
	maxCallerTokenBytes = 16 * 1024
	// callerTokenMaxAge re-reads the file even when its metadata looks
	// unchanged. Kubelet rotates the projected token at 80% of its lifetime
	// (at least 8 of 10 minutes), so the cached copy is always still valid.
	callerTokenMaxAge = 30 * time.Second
)

// WithCallerTokenFile attaches the token in path to every RPC on this client
// only. It fails if the file is unreadable now, so a misconfigured deployment
// never starts and silently sends anonymous calls.
func WithCallerTokenFile(path string) (Option, error) {
	creds, err := newCallerTokenCredentials(path)
	if err != nil {
		return nil, err
	}
	return WithDialOption(grpc.WithPerRPCCredentials(creds)), nil
}

// callerTokenCredentials re-reads the kubelet-rotated token file when it
// changes (projected volumes swap a symlink, so the file identity changes) or
// when the cached copy is older than callerTokenMaxAge. An unreadable, empty
// or oversized file fails the RPC as Unauthenticated instead of sending it
// without a token.
type callerTokenCredentials struct {
	path string
	now  func() time.Time

	mu       sync.Mutex
	token    string
	info     os.FileInfo
	loadedAt time.Time
}

var _ credentials.PerRPCCredentials = (*callerTokenCredentials)(nil)

func newCallerTokenCredentials(path string) (*callerTokenCredentials, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("caller token file path is required")
	}
	c := &callerTokenCredentials{path: path, now: time.Now}
	if _, err := c.current(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *callerTokenCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	token, err := c.current()
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "runners caller token: %v", err)
	}
	return map[string]string{CallerTokenMetadataKey: token}, nil
}

// RequireTransportSecurity is false: the platform's internal gRPC is
// plaintext h2c. The token is audience-bound to Runners and pod-bound.
func (c *callerTokenCredentials) RequireTransportSecurity() bool {
	return false
}

func (c *callerTokenCredentials) current() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Stat(c.path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", c.path, err)
	}
	if c.token != "" && c.info != nil && os.SameFile(c.info, info) &&
		c.info.ModTime().Equal(info.ModTime()) && c.info.Size() == info.Size() &&
		c.now().Sub(c.loadedAt) < callerTokenMaxAge {
		return c.token, nil
	}
	token, err := readCallerToken(c.path)
	if err != nil {
		return "", err
	}
	c.token, c.info, c.loadedAt = token, info, c.now()
	return token, nil
}

func readCallerToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCallerTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxCallerTokenBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", path, maxCallerTokenBytes)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return token, nil
}
