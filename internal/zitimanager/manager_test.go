package zitimanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zitimgmtv1 "github.com/agynio/gateway/gen/agynio/api/ziti_management/v1"
	"github.com/agynio/gateway/internal/zitimgmtclient"
)

type fakeZitiManagementServer struct {
	zitimgmtv1.UnimplementedZitiManagementServiceServer
	requestServiceIdentity func(context.Context, *zitimgmtv1.RequestServiceIdentityRequest) (*zitimgmtv1.RequestServiceIdentityResponse, error)
	extendIdentityLease    func(context.Context, *zitimgmtv1.ExtendIdentityLeaseRequest) (*zitimgmtv1.ExtendIdentityLeaseResponse, error)
}

func (f *fakeZitiManagementServer) RequestServiceIdentity(ctx context.Context, req *zitimgmtv1.RequestServiceIdentityRequest) (*zitimgmtv1.RequestServiceIdentityResponse, error) {
	if f.requestServiceIdentity != nil {
		return f.requestServiceIdentity(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeZitiManagementServer) ExtendIdentityLease(ctx context.Context, req *zitimgmtv1.ExtendIdentityLeaseRequest) (*zitimgmtv1.ExtendIdentityLeaseResponse, error) {
	if f.extendIdentityLease != nil {
		return f.extendIdentityLease(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

type fakeZitiContext struct {
	ziti.Context
	mu     sync.Mutex
	closed bool
}

func (f *fakeZitiContext) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeZitiContext) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// fakeListener stands in for the SDK multi-listener returned by ListenWithOptions.
type fakeListener struct {
	established  atomic.Uint32
	closed       atomic.Bool
	mu           sync.Mutex
	errorHandler func(error)
}

func (l *fakeListener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}

func (l *fakeListener) Close() error {
	l.closed.Store(true)
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

func (l *fakeListener) GetEstablishedCount() uint {
	return uint(l.established.Load())
}

func (l *fakeListener) SetErrorEventHandler(handler func(error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errorHandler = handler
}

func (l *fakeListener) ErrorHandler() func(error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.errorHandler
}

func (l *fakeListener) Closed() bool {
	return l.closed.Load()
}

type bindFunc func(call int, listener *fakeListener, options *ziti.ListenOptions) (net.Listener, error)

// waitForEstablished mimics ListenWithOptions with WaitForNEstablishedListeners:
// it returns once the fake router confirms a terminator, or closes the listener
// and fails after options.ConnectTimeout.
func waitForEstablished(establish <-chan struct{}) bindFunc {
	return func(_ int, listener *fakeListener, options *ziti.ListenOptions) (net.Listener, error) {
		if options.WaitForNEstablishedListeners == 0 {
			return listener, nil
		}
		select {
		case <-establish:
			listener.established.Store(uint32(options.WaitForNEstablishedListeners))
			return listener, nil
		case <-time.After(options.ConnectTimeout):
			_ = listener.Close()
			return nil, fmt.Errorf("timed out waiting for %d listeners to be established, only had 0", options.WaitForNEstablishedListeners)
		}
	}
}

// managerFixture issues identity-1, identity-2, ... from a fake ziti-management
// server and records contexts, listeners and the order of lease extensions and
// listener hand-offs.
type managerFixture struct {
	t      *testing.T
	client *zitimgmtclient.Client

	// Optional behavior, set before the manager starts. Calls are 1-based;
	// extendErr counts calls per identity.
	requestErr func(call int) error
	extendErr  func(identityID string, call int) error
	bind       bindFunc

	newListenerCh chan net.Listener

	mu           sync.Mutex
	requestTimes []time.Time
	issued       int
	extendCalls  map[string]int
	events       []string
	contexts     []*fakeZitiContext
	boundCtxs    []ziti.Context
	bindOptions  []*ziti.ListenOptions
	listeners    []*fakeListener
	established  []uint
	// readyAtHandoff is the manager's EstablishedListeners() seen inside
	// onNewListener.
	readyAtHandoff []uint
	mgr            *Manager
}

func newManagerFixture(t *testing.T) *managerFixture {
	t.Helper()

	f := &managerFixture{
		t:             t,
		extendCalls:   map[string]int{},
		newListenerCh: make(chan net.Listener, 16),
	}
	server := &fakeZitiManagementServer{
		requestServiceIdentity: func(ctx context.Context, req *zitimgmtv1.RequestServiceIdentityRequest) (*zitimgmtv1.RequestServiceIdentityResponse, error) {
			if req.ServiceType != zitimgmtv1.ServiceType_SERVICE_TYPE_GATEWAY {
				return nil, status.Errorf(codes.InvalidArgument, "unexpected service type %v", req.ServiceType)
			}
			f.mu.Lock()
			f.requestTimes = append(f.requestTimes, time.Now())
			call := len(f.requestTimes)
			f.mu.Unlock()
			if f.requestErr != nil {
				if err := f.requestErr(call); err != nil {
					return nil, err
				}
			}
			f.mu.Lock()
			f.issued++
			identityID := fmt.Sprintf("identity-%d", f.issued)
			f.mu.Unlock()
			return &zitimgmtv1.RequestServiceIdentityResponse{
				ZitiIdentityId: identityID,
				IdentityJson:   []byte("{}"),
			}, nil
		},
		extendIdentityLease: func(ctx context.Context, req *zitimgmtv1.ExtendIdentityLeaseRequest) (*zitimgmtv1.ExtendIdentityLeaseResponse, error) {
			f.mu.Lock()
			f.extendCalls[req.ZitiIdentityId]++
			call := f.extendCalls[req.ZitiIdentityId]
			f.events = append(f.events, "extend:"+req.ZitiIdentityId)
			f.mu.Unlock()
			if f.extendErr != nil {
				if err := f.extendErr(req.ZitiIdentityId, call); err != nil {
					return nil, err
				}
			}
			return &zitimgmtv1.ExtendIdentityLeaseResponse{}, nil
		},
	}

	addr, stop := startZitiManagementServer(t, server)
	t.Cleanup(stop)

	client, err := zitimgmtclient.NewClient(addr)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	f.client = client

	originalFactory := newZitiContext
	newZitiContext = func(config *ziti.Config) (ziti.Context, error) {
		zitiCtx := &fakeZitiContext{}
		f.mu.Lock()
		f.contexts = append(f.contexts, zitiCtx)
		f.mu.Unlock()
		return zitiCtx, nil
	}
	t.Cleanup(func() {
		newZitiContext = originalFactory
	})

	setLeaseRetryBackoffs(t, []time.Duration{time.Millisecond})
	setReEnrollBackoffs(t, []time.Duration{5 * time.Millisecond})

	return f
}

func (f *managerFixture) listenerFactory(zitiCtx ziti.Context, options *ziti.ListenOptions) (net.Listener, error) {
	listener := &fakeListener{}
	f.mu.Lock()
	f.listeners = append(f.listeners, listener)
	f.boundCtxs = append(f.boundCtxs, zitiCtx)
	f.bindOptions = append(f.bindOptions, options)
	call := len(f.listeners)
	f.mu.Unlock()

	if f.bind == nil {
		listener.established.Store(1)
		return listener, nil
	}
	return f.bind(call, listener, options)
}

func (f *managerFixture) onNewListener(listener net.Listener, established uint) {
	f.mu.Lock()
	index := 0
	for i, candidate := range f.listeners {
		if net.Listener(candidate) == listener {
			index = i + 1
		}
	}
	f.events = append(f.events, fmt.Sprintf("listener:%d", index))
	f.established = append(f.established, established)
	if f.mgr != nil {
		f.readyAtHandoff = append(f.readyAtHandoff, f.mgr.EstablishedListeners())
	}
	f.mu.Unlock()
	f.newListenerCh <- listener
}

func (f *managerFixture) newManager(enrollTimeout, renewalInterval, bindTimeout time.Duration) *Manager {
	f.t.Helper()

	mgr, err := New(
		f.client,
		zitimgmtv1.ServiceType_SERVICE_TYPE_GATEWAY,
		enrollTimeout,
		renewalInterval,
		bindTimeout,
		f.listenerFactory,
		f.onNewListener,
	)
	if err != nil {
		f.t.Fatalf("failed to create manager: %v", err)
	}
	f.mu.Lock()
	f.mgr = mgr
	f.mu.Unlock()
	return mgr
}

func (f *managerFixture) context(n int) *fakeZitiContext {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if n < 1 || n > len(f.contexts) {
		f.t.Fatalf("expected ziti context %d, have %d", n, len(f.contexts))
	}
	return f.contexts[n-1]
}

func (f *managerFixture) listener(n int) *fakeListener {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if n < 1 || n > len(f.listeners) {
		f.t.Fatalf("expected listener %d, have %d", n, len(f.listeners))
	}
	return f.listeners[n-1]
}

func (f *managerFixture) allContexts() []*fakeZitiContext {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeZitiContext(nil), f.contexts...)
}

func (f *managerFixture) allListeners() []*fakeListener {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeListener(nil), f.listeners...)
}

func (f *managerFixture) requests() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.requestTimes...)
}

func (f *managerFixture) eventLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...)
}

func (f *managerFixture) awaitNewListener(what string) net.Listener {
	f.t.Helper()
	select {
	case listener := <-f.newListenerCh:
		return listener
	case <-time.After(2 * time.Second):
		f.t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

func (f *managerFixture) expectNoNewListener() {
	f.t.Helper()
	select {
	case listener := <-f.newListenerCh:
		f.t.Fatalf("unexpected listener hand-off: %v", listener)
	default:
	}
}

func startZitiManagementServer(t *testing.T, server *fakeZitiManagementServer) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	grpcServer := grpc.NewServer()
	zitimgmtv1.RegisterZitiManagementServiceServer(grpcServer, server)
	go func() {
		_ = grpcServer.Serve(listener)
	}()

	return listener.Addr().String(), func() {
		grpcServer.Stop()
		_ = listener.Close()
	}
}

func setLeaseRetryBackoffs(t *testing.T, backoffs []time.Duration) {
	t.Helper()

	original := leaseRetryBackoffs
	leaseRetryBackoffs = backoffs
	t.Cleanup(func() {
		leaseRetryBackoffs = original
	})
}

func setReEnrollBackoffs(t *testing.T, backoffs []time.Duration) {
	t.Helper()

	original := reEnrollBackoffs
	reEnrollBackoffs = backoffs
	t.Cleanup(func() {
		reEnrollBackoffs = original
	})
}

func setWatchdogInterval(t *testing.T, interval time.Duration) {
	t.Helper()

	original := watchdogInterval
	watchdogInterval = interval
	t.Cleanup(func() {
		watchdogInterval = original
	})
}

func setBindErrorLogInterval(t *testing.T, interval time.Duration) {
	t.Helper()

	original := bindErrorLogInterval
	bindErrorLogInterval = interval
	t.Cleanup(func() {
		bindErrorLogInterval = original
	})
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()

	buffer := &syncBuffer{}
	original := log.Writer()
	log.SetOutput(buffer)
	t.Cleanup(func() {
		log.SetOutput(original)
	})
	return buffer
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// runManager runs mgr in the background; the returned stop cancels it and
// returns Run's result.
func runManager(t *testing.T, mgr *Manager) func() error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mgr.Run(ctx)
	}()
	var once sync.Once
	var runErr error
	stop := func() error {
		once.Do(func() {
			cancel()
			runErr = <-done
		})
		return runErr
	}
	t.Cleanup(func() {
		_ = stop()
	})
	return stop
}

func TestNewRejectsNonPositiveBindTimeout(t *testing.T) {
	f := newManagerFixture(t)

	_, err := New(
		f.client,
		zitimgmtv1.ServiceType_SERVICE_TYPE_GATEWAY,
		time.Second,
		time.Second,
		0,
		f.listenerFactory,
		f.onNewListener,
	)
	if err == nil {
		t.Fatalf("expected zero bind timeout to be rejected")
	}
}

func TestManagerStartEnrollsAndCreatesListener(t *testing.T) {
	f := newManagerFixture(t)
	mgr := f.newManager(time.Second, time.Hour, 750*time.Millisecond)

	if mgr.EstablishedListeners() != 0 {
		t.Fatalf("expected no established listeners before start")
	}
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	got := f.awaitNewListener("initial listener")
	listener := f.listener(1)
	if got != net.Listener(listener) {
		t.Fatalf("expected listener callback to receive listener")
	}
	if f.boundCtxs[0] != ziti.Context(f.context(1)) {
		t.Fatalf("expected listener factory to receive ziti context")
	}
	options := f.bindOptions[0]
	if options.WaitForNEstablishedListeners != 1 {
		t.Fatalf("expected bind to wait for 1 established listener, got %d", options.WaitForNEstablishedListeners)
	}
	if options.ConnectTimeout != 750*time.Millisecond {
		t.Fatalf("expected bind timeout %s, got %s", 750*time.Millisecond, options.ConnectTimeout)
	}
	if f.established[0] != 1 {
		t.Fatalf("expected callback to report 1 established listener, got %d", f.established[0])
	}
	if f.readyAtHandoff[0] != 1 {
		t.Fatalf("expected manager to report ready during hand-off, got %d", f.readyAtHandoff[0])
	}
	if mgr.ZitiContext() != ziti.Context(f.context(1)) {
		t.Fatalf("expected ziti context to be stored")
	}
	if mgr.identity() != "identity-1" {
		t.Fatalf("expected identity id %q, got %q", "identity-1", mgr.identity())
	}
	if mgr.EstablishedListeners() != 1 {
		t.Fatalf("expected 1 established listener, got %d", mgr.EstablishedListeners())
	}
	if listener.ErrorHandler() == nil {
		t.Fatalf("expected child bind error handler to be installed")
	}
	if f.context(1).Closed() || listener.Closed() {
		t.Fatalf("expected active context and listener to stay open")
	}
}

func TestManagerStartWaitsForDelayedBind(t *testing.T) {
	f := newManagerFixture(t)
	establish := make(chan struct{})
	bindStarted := make(chan struct{})
	var bindStartedOnce sync.Once
	waitForRouter := waitForEstablished(establish)
	f.bind = func(call int, listener *fakeListener, options *ziti.ListenOptions) (net.Listener, error) {
		bindStartedOnce.Do(func() {
			close(bindStarted)
		})
		return waitForRouter(call, listener, options)
	}
	mgr := f.newManager(5*time.Second, time.Hour, 2*time.Second)

	startErr := make(chan error, 1)
	go func() {
		startErr <- mgr.Start(context.Background())
	}()

	select {
	case <-bindStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected bind to start")
	}
	// Routers keep rejecting binds until they learn the identity; the gateway
	// must neither hand off the listener nor report ready meanwhile.
	time.Sleep(50 * time.Millisecond)
	if mgr.EstablishedListeners() != 0 || mgr.ZitiContext() != nil {
		t.Fatalf("expected manager to stay not ready while bind is pending")
	}
	f.expectNoNewListener()

	close(establish)
	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("expected delayed bind to succeed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected start to return after bind")
	}

	if got := f.awaitNewListener("established listener"); got != net.Listener(f.listener(1)) {
		t.Fatalf("expected established listener to be handed off")
	}
	if mgr.EstablishedListeners() != 1 {
		t.Fatalf("expected 1 established listener, got %d", mgr.EstablishedListeners())
	}
	if got := len(f.requests()); got != 1 {
		t.Fatalf("expected a single enrollment, got %d", got)
	}
}

func TestManagerStartFailsClosedWhenBindNeverEstablishes(t *testing.T) {
	f := newManagerFixture(t)
	f.bind = waitForEstablished(make(chan struct{}))
	mgr := f.newManager(200*time.Millisecond, time.Hour, 40*time.Millisecond)

	if err := mgr.Start(context.Background()); err == nil {
		t.Fatalf("expected start to fail when no terminator is established")
	}

	if got := len(f.requests()); got < 2 {
		t.Fatalf("expected enrollment to be retried within the enrollment timeout, got %d attempts", got)
	}
	for i, zitiCtx := range f.allContexts() {
		if !zitiCtx.Closed() {
			t.Fatalf("expected ziti context %d to be closed", i+1)
		}
	}
	waitFor(t, "unestablished listeners to close", func() bool {
		for _, listener := range f.allListeners() {
			if !listener.Closed() {
				return false
			}
		}
		return true
	})
	if mgr.ZitiContext() != nil || mgr.identity() != "" || mgr.EstablishedListeners() != 0 {
		t.Fatalf("expected manager state to be cleared")
	}
	f.expectNoNewListener()
	if events := f.eventLog(); len(events) != 0 {
		t.Fatalf("expected no lease extension or hand-off, got %v", events)
	}
}

func TestManagerStartBoundsBindThatNeverReturns(t *testing.T) {
	f := newManagerFixture(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseBinds := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	t.Cleanup(releaseBinds)
	f.bind = func(_ int, listener *fakeListener, _ *ziti.ListenOptions) (net.Listener, error) {
		<-release
		listener.established.Store(1)
		return listener, nil
	}
	mgr := f.newManager(100*time.Millisecond, time.Hour, 30*time.Millisecond)

	started := time.Now()
	if err := mgr.Start(context.Background()); err == nil {
		t.Fatalf("expected start to fail when the bind never returns")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("expected start to be bounded by the enrollment timeout, took %s", elapsed)
	}
	for i, zitiCtx := range f.allContexts() {
		if !zitiCtx.Closed() {
			t.Fatalf("expected ziti context %d to be closed", i+1)
		}
	}
	if mgr.ZitiContext() != nil || mgr.EstablishedListeners() != 0 {
		t.Fatalf("expected manager state to be cleared")
	}

	releaseBinds()
	waitFor(t, "late listeners to close", func() bool {
		for _, listener := range f.allListeners() {
			if !listener.Closed() {
				return false
			}
		}
		return true
	})
	f.expectNoNewListener()
}

func TestManagerStartRejectsListenerWithoutEstablishedTerminators(t *testing.T) {
	f := newManagerFixture(t)
	f.bind = func(call int, listener *fakeListener, _ *ziti.ListenOptions) (net.Listener, error) {
		if call > 1 {
			listener.established.Store(1)
		}
		return listener, nil
	}
	mgr := f.newManager(time.Second, time.Hour, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	if got := f.awaitNewListener("established listener"); got != net.Listener(f.listener(2)) {
		t.Fatalf("expected only the established listener to be handed off")
	}
	f.expectNoNewListener()
	if !f.listener(1).Closed() || !f.context(1).Closed() {
		t.Fatalf("expected unestablished listener and its context to be closed")
	}
	if events := f.eventLog(); strings.Join(events, ",") != "extend:identity-2,listener:2" {
		t.Fatalf("expected only the established identity to be extended, got %v", events)
	}
	if mgr.identity() != "identity-2" || mgr.ZitiContext() != ziti.Context(f.context(2)) {
		t.Fatalf("expected manager to keep the established identity")
	}
}

func TestManagerStartExtendsLeaseOnceBeforeOnNewListener(t *testing.T) {
	f := newManagerFixture(t)
	mgr := f.newManager(time.Second, time.Hour, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")

	if events := f.eventLog(); strings.Join(events, ",") != "extend:identity-1,listener:1" {
		t.Fatalf("expected one lease extension before the listener hand-off, got %v", events)
	}
}

func TestManagerStartReenrollsWhenPostBindExtensionNotFound(t *testing.T) {
	f := newManagerFixture(t)
	f.extendErr = func(identityID string, _ int) error {
		if identityID == "identity-1" {
			return status.Error(codes.NotFound, "identity lease expired")
		}
		return nil
	}
	mgr := f.newManager(time.Second, time.Hour, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	if got := f.awaitNewListener("re-enrolled listener"); got != net.Listener(f.listener(2)) {
		t.Fatalf("expected the re-enrolled listener to be handed off")
	}
	f.expectNoNewListener()
	if events := f.eventLog(); strings.Join(events, ",") != "extend:identity-1,extend:identity-2,listener:2" {
		t.Fatalf("unexpected lease and hand-off order: %v", events)
	}
	if !f.listener(1).Closed() || !f.context(1).Closed() {
		t.Fatalf("expected the expired identity's listener and context to be closed")
	}
	if mgr.identity() != "identity-2" || mgr.ZitiContext() != ziti.Context(f.context(2)) {
		t.Fatalf("expected manager to keep the re-enrolled identity")
	}
}

func TestRunWatchdogReenrollsAfterLosingTerminators(t *testing.T) {
	setWatchdogInterval(t, 5*time.Millisecond)
	f := newManagerFixture(t)
	mgr := f.newManager(time.Second, time.Hour, 50*time.Millisecond)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")
	stop := runManager(t, mgr)

	lostAt := time.Now()
	f.listener(1).established.Store(0)
	if mgr.EstablishedListeners() != 0 {
		t.Fatalf("expected readiness to drop with the terminators")
	}

	if got := f.awaitNewListener("re-enrolled listener"); got != net.Listener(f.listener(2)) {
		t.Fatalf("expected the re-enrolled listener to be handed off")
	}
	requests := f.requests()
	if len(requests) != 2 {
		t.Fatalf("expected one re-enrollment, got %d enrollments", len(requests))
	}
	if delay := requests[1].Sub(lostAt); delay < 50*time.Millisecond {
		t.Fatalf("expected re-enrollment only after the bind timeout, got %s", delay)
	}
	waitFor(t, "re-enrolled listener to be ready", func() bool {
		return mgr.EstablishedListeners() == 1
	})
	if !f.listener(1).Closed() || !f.context(1).Closed() {
		t.Fatalf("expected the lost listener and its context to be closed")
	}
	if mgr.ZitiContext() != ziti.Context(f.context(2)) {
		t.Fatalf("expected manager to store the new ziti context")
	}

	if err := stop(); err != nil {
		t.Fatalf("expected run to stop cleanly: %v", err)
	}
}

func TestRunWatchdogToleratesRecoveryWithinBindTimeout(t *testing.T) {
	setWatchdogInterval(t, 5*time.Millisecond)
	f := newManagerFixture(t)
	mgr := f.newManager(time.Second, time.Hour, 300*time.Millisecond)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")
	runManager(t, mgr)

	f.listener(1).established.Store(0)
	time.Sleep(50 * time.Millisecond)
	f.listener(1).established.Store(1)
	time.Sleep(400 * time.Millisecond)

	if got := len(f.requests()); got != 1 {
		t.Fatalf("expected no re-enrollment after recovery, got %d enrollments", got)
	}
	if f.context(1).Closed() || mgr.EstablishedListeners() != 1 {
		t.Fatalf("expected the recovered listener to stay active")
	}
}

func TestRunFailsClosedWhenReenrollmentFails(t *testing.T) {
	f := newManagerFixture(t)
	f.requestErr = func(call int) error {
		if call > 1 {
			return status.Error(codes.PermissionDenied, "denied")
		}
		return nil
	}
	f.extendErr = func(_ string, call int) error {
		if call > 1 {
			return status.Error(codes.NotFound, "missing")
		}
		return nil
	}
	mgr := f.newManager(100*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Run(ctx); err == nil {
		t.Fatalf("expected run to fail when re-enrollment cannot complete")
	}
	if ctx.Err() != nil {
		t.Fatalf("expected run to fail within the enrollment timeout")
	}

	if got := len(f.requests()); got < 2 {
		t.Fatalf("expected re-enrollment attempts, got %d enrollments", got)
	}
	if !f.listener(1).Closed() || !f.context(1).Closed() {
		t.Fatalf("expected the lost identity's listener and context to be closed")
	}
	if mgr.ZitiContext() != nil || mgr.EstablishedListeners() != 0 {
		t.Fatalf("expected manager state to be cleared")
	}
}

func TestRunLeaseRenewalReenrollsOnNotFound(t *testing.T) {
	f := newManagerFixture(t)
	f.extendErr = func(identityID string, call int) error {
		if identityID == "identity-1" && call > 1 {
			return status.Error(codes.NotFound, "missing")
		}
		return nil
	}
	mgr := f.newManager(time.Second, 10*time.Millisecond, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")
	runManager(t, mgr)

	if got := f.awaitNewListener("re-enrolled listener"); got != net.Listener(f.listener(2)) {
		t.Fatalf("expected the re-enrolled listener to be handed off")
	}
	waitFor(t, "manager to store the new ziti context", func() bool {
		return mgr.ZitiContext() == ziti.Context(f.context(2))
	})
	if !f.context(1).Closed() {
		t.Fatalf("expected old ziti context to be closed")
	}
	if got := len(f.requests()); got != 2 {
		t.Fatalf("expected one re-enrollment, got %d enrollments", got)
	}
}

func TestRunLeaseRenewalReenrollsAfterFailureWithBackoff(t *testing.T) {
	f := newManagerFixture(t)
	setReEnrollBackoffs(t, []time.Duration{20 * time.Millisecond})
	f.requestErr = func(call int) error {
		if call == 2 {
			return status.Error(codes.PermissionDenied, "denied")
		}
		return nil
	}
	f.extendErr = func(identityID string, call int) error {
		if identityID == "identity-1" && call > 1 {
			return status.Error(codes.NotFound, "missing")
		}
		return nil
	}
	mgr := f.newManager(time.Second, 5*time.Millisecond, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")
	runManager(t, mgr)

	if got := f.awaitNewListener("re-enrolled listener"); got != net.Listener(f.listener(2)) {
		t.Fatalf("expected the re-enrolled listener to be handed off")
	}
	requests := f.requests()
	if len(requests) != 3 {
		t.Fatalf("expected 3 enrollment requests, got %d", len(requests))
	}
	if delta := requests[2].Sub(requests[1]); delta < 15*time.Millisecond {
		t.Fatalf("expected re-enroll backoff, got %s", delta)
	}
	waitFor(t, "manager to store the new ziti context", func() bool {
		return mgr.ZitiContext() == ziti.Context(f.context(2))
	})
	if !f.context(1).Closed() {
		t.Fatalf("expected old ziti context to be closed")
	}
}

func TestExtendLeaseWithRetryRetriesTransientErrors(t *testing.T) {
	f := newManagerFixture(t)
	setLeaseRetryBackoffs(t, []time.Duration{time.Millisecond, time.Millisecond})
	f.extendErr = func(_ string, call int) error {
		if call < 3 {
			return status.Error(codes.Unavailable, "unavailable")
		}
		return nil
	}
	mgr := f.newManager(time.Second, time.Second, time.Second)

	if err := mgr.extendLeaseWithRetry(context.Background(), "identity-1"); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if got := len(f.eventLog()); got != 3 {
		t.Fatalf("expected 3 extend attempts, got %d", got)
	}
}

func TestExtendLeaseWithRetryStopsOnNonRetryable(t *testing.T) {
	f := newManagerFixture(t)
	setLeaseRetryBackoffs(t, []time.Duration{time.Millisecond, time.Millisecond})
	f.extendErr = func(string, int) error {
		return status.Error(codes.PermissionDenied, "denied")
	}
	mgr := f.newManager(time.Second, time.Second, time.Second)

	err := mgr.extendLeaseWithRetry(context.Background(), "identity-1")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if got := len(f.eventLog()); got != 1 {
		t.Fatalf("expected 1 extend attempt, got %d", got)
	}
}

func TestManagerLogsChildBindErrors(t *testing.T) {
	setBindErrorLogInterval(t, time.Hour)
	f := newManagerFixture(t)
	mgr := f.newManager(time.Second, time.Hour, time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}
	f.awaitNewListener("initial listener")
	handler := f.listener(1).ErrorHandler()
	if handler == nil {
		t.Fatalf("expected child bind error handler to be installed")
	}

	output := captureLog(t)
	bindErr := errors.New("attempt to use closed connection: identity not found by id")
	for i := 0; i < 3; i++ {
		handler(bindErr)
	}

	logged := output.String()
	if got := strings.Count(logged, "ziti service listener bind error"); got != 1 {
		t.Fatalf("expected repeated bind errors to be throttled to 1 line, got %d: %q", got, logged)
	}
	if !strings.Contains(logged, "identity not found by id") {
		t.Fatalf("expected bind error cause to be logged, got %q", logged)
	}
}

func TestBindErrorLoggerSummarizesSuppressedErrors(t *testing.T) {
	setBindErrorLogInterval(t, 20*time.Millisecond)
	output := captureLog(t)
	logger := &bindErrorLogger{}
	bindErr := errors.New("identity not found by id")

	for i := 0; i < 3; i++ {
		logger.log(bindErr)
	}
	time.Sleep(30 * time.Millisecond)
	logger.log(bindErr)

	logged := output.String()
	if got := strings.Count(logged, "ziti service listener bind error"); got != 2 {
		t.Fatalf("expected 2 logged bind errors, got %d: %q", got, logged)
	}
	if !strings.Contains(logged, "(2 earlier errors suppressed)") {
		t.Fatalf("expected suppressed errors to be summarized, got %q", logged)
	}
}
