package grpcclient

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	runnerv1 "github.com/agynio/gateway/gen/agynio/api/runner/v1"
	runnersv1 "github.com/agynio/gateway/gen/agynio/api/runners/v1"
	threadsv1 "github.com/agynio/gateway/gen/agynio/api/threads/v1"
	"github.com/agynio/gateway/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// writeProjectedToken mimics kubelet's atomic writer: the token path is a
// symlink into a timestamped directory that is swapped on rotation.
func writeProjectedToken(t *testing.T, dir, token string) string {
	t.Helper()
	data := filepath.Join(dir, "..data-"+strings.ReplaceAll(t.Name(), "/", "_")+"-"+time.Now().Format("150405.000000000"))
	if err := os.MkdirAll(data, 0o755); err != nil {
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

func TestCallerTokenCredentialsHeaderAndRotation(t *testing.T) {
	dir := t.TempDir()
	path := writeProjectedToken(t, dir, "first.token.v1\n")
	creds, err := newCallerTokenCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if creds.RequireTransportSecurity() {
		t.Fatal("caller token must work over the platform's plaintext h2c")
	}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil || md[CallerTokenMetadataKey] != "first.token.v1" || len(md) != 1 {
		t.Fatalf("metadata %v %v", md, err)
	}
	writeProjectedToken(t, dir, "second.token.v2")
	md, err = creds.GetRequestMetadata(context.Background())
	if err != nil || md[CallerTokenMetadataKey] != "second.token.v2" {
		t.Fatalf("rotated token not picked up: %v %v", md, err)
	}
}

func TestCallerTokenCredentialsRereadsAfterMaxAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("a.b.c"), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, err := newCallerTokenCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	creds.now = func() time.Time { return now }
	stat, _ := os.Stat(path)
	// Same inode, size and mtime: only the age forces a re-read.
	if err := os.WriteFile(path, []byte("d.e.f"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	creds.loadedAt = now
	if md, _ := creds.GetRequestMetadata(context.Background()); md[CallerTokenMetadataKey] != "a.b.c" {
		t.Fatalf("re-read before max age: %v", md)
	}
	now = now.Add(callerTokenMaxAge)
	if md, _ := creds.GetRequestMetadata(context.Background()); md[CallerTokenMetadataKey] != "d.e.f" {
		t.Fatalf("no re-read after max age: %v", md)
	}
}

func TestCallerTokenFileRejectsUnusableFiles(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	_ = os.WriteFile(empty, []byte(" \n"), 0o600)
	big := filepath.Join(dir, "big")
	_ = os.WriteFile(big, []byte(strings.Repeat("a", maxCallerTokenBytes+1)), 0o600)
	for _, path := range []string{"", filepath.Join(dir, "missing"), empty, big} {
		if _, err := WithCallerTokenFile(path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
}

type tokenCaptureRunners struct {
	runnersv1.UnimplementedRunnersServiceServer
	mu    sync.Mutex
	calls []metadata.MD
}

func (s *tokenCaptureRunners) capture(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, md)
}

func (s *tokenCaptureRunners) GetWorkload(ctx context.Context, _ *runnersv1.GetWorkloadRequest) (*runnersv1.GetWorkloadResponse, error) {
	s.capture(ctx)
	return &runnersv1.GetWorkloadResponse{Workload: &runnersv1.Workload{}}, nil
}

func (s *tokenCaptureRunners) StreamWorkloadLogs(_ *runnerv1.StreamWorkloadLogsRequest, stream grpc.ServerStreamingServer[runnerv1.StreamWorkloadLogsResponse]) error {
	s.capture(stream.Context())
	return nil
}

type tokenCaptureThreads struct {
	threadsv1.UnimplementedThreadsServiceServer
	md metadata.MD
}

func (s *tokenCaptureThreads) GetThread(ctx context.Context, _ *threadsv1.GetThreadRequest) (*threadsv1.GetThreadResponse, error) {
	s.md, _ = metadata.FromIncomingContext(ctx)
	return &threadsv1.GetThreadResponse{}, nil
}

func bufDialer(listener *bufconn.Listener) Option {
	return WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() })
}

// Only the Runners client carries the token, on unary and streaming calls,
// next to the forwarded identity; another client on the same process never does.
func TestCallerTokenOnlyOnRunnersClient(t *testing.T) {
	listener := bufconn.Listen(bufConnSize)
	server := grpc.NewServer()
	runners := &tokenCaptureRunners{}
	threads := &tokenCaptureThreads{}
	runnersv1.RegisterRunnersServiceServer(server, runners)
	threadsv1.RegisterThreadsServiceServer(server, threads)
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	tokenPath := writeProjectedToken(t, t.TempDir(), "runners.caller.token")
	tokenOption, err := WithCallerTokenFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	runnersClient, err := New("passthrough:///bufnet", runnersv1.NewRunnersServiceClient, bufDialer(listener), tokenOption)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runnersClient.Close() })
	threadsClient, err := New("passthrough:///bufnet", threadsv1.NewThreadsServiceClient, bufDialer(listener))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = threadsClient.Close() })

	ctx := identity.WithIdentity(context.Background(), identity.ResolvedIdentity{IdentityID: "identity-1", IdentityType: identity.IdentityTypeUser})
	if _, err := runnersClient.Service().GetWorkload(ctx, &runnersv1.GetWorkloadRequest{Id: "w"}); err != nil {
		t.Fatal(err)
	}
	stream, err := runnersClient.Service().StreamWorkloadLogs(ctx, &runnerv1.StreamWorkloadLogsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err == nil {
		t.Fatal("expected end of stream")
	}
	if _, err := threadsClient.Service().GetThread(ctx, &threadsv1.GetThreadRequest{}); err != nil {
		t.Fatal(err)
	}

	if len(runners.calls) != 2 {
		t.Fatalf("runners calls %d", len(runners.calls))
	}
	for _, md := range runners.calls {
		assertMetadataValues(t, md, CallerTokenMetadataKey, []string{"runners.caller.token"})
		assertMetadataValues(t, md, identity.MetadataKeyIdentityID, []string{"identity-1"})
	}
	assertMetadataValues(t, threads.md, CallerTokenMetadataKey, nil)
	assertMetadataValues(t, threads.md, identity.MetadataKeyIdentityID, []string{"identity-1"})
}

// A token that disappears after startup fails the call instead of sending it
// anonymously; the server never sees the request.
func TestCallerTokenMissingFailsClosed(t *testing.T) {
	listener := bufconn.Listen(bufConnSize)
	server := grpc.NewServer()
	runners := &tokenCaptureRunners{}
	runnersv1.RegisterRunnersServiceServer(server, runners)
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	dir := t.TempDir()
	tokenPath := writeProjectedToken(t, dir, "runners.caller.token")
	tokenOption, err := WithCallerTokenFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := New("passthrough:///bufnet", runnersv1.NewRunnersServiceClient, bufDialer(listener), tokenOption)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := os.Remove(tokenPath); err != nil {
		t.Fatal(err)
	}
	_, err = client.Service().GetWorkload(context.Background(), &runnersv1.GetWorkloadRequest{Id: "w"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing token: %v, want Unauthenticated", err)
	}
	if len(runners.calls) != 0 {
		t.Fatal("server received a call without a caller token")
	}
}
