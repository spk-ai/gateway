package zitimanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zitimgmtv1 "github.com/agynio/gateway/gen/agynio/api/ziti_management/v1"
	"github.com/agynio/gateway/internal/zitimgmtclient"
)

const (
	retryInitialBackoff = 1 * time.Second
	retryMaxBackoff     = 15 * time.Second

	// requiredEstablishedListeners is the number of router-confirmed terminators
	// the service listener needs before the gateway is reachable over Ziti.
	requiredEstablishedListeners = 1
)

var (
	leaseRetryBackoffs = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	// leaseExtendAttemptTimeout bounds each ExtendIdentityLease call; the
	// management client sets no deadline of its own.
	leaseExtendAttemptTimeout = 10 * time.Second
	reEnrollBackoffs          = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	// reEnrollRoundBackoffs space failed runtime re-enrollment rounds; the last
	// entry repeats.
	reEnrollRoundBackoffs = []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute}
	// watchdogInterval is how often Run samples the established terminator count.
	watchdogInterval = 5 * time.Second
	// bindErrorLogInterval throttles child bind errors: the SDK retries a rejected
	// bind every few milliseconds, so later errors are summarized.
	bindErrorLogInterval = 10 * time.Second
	newZitiContext       = ziti.NewContext
	errIdentityMissing   = errors.New("ziti identity id missing")
	errNotEstablished    = errors.New("ziti service listener has no established terminators")
)

// ListenerFactory binds the service using options owned by the Manager. It must
// pass them through unchanged: they make the bind wait for a router-confirmed
// terminator for at most the bind timeout.
type ListenerFactory func(zitiCtx ziti.Context, options *ziti.ListenOptions) (net.Listener, error)

// OnNewListener receives a listener only after at least one terminator is
// established and an extension of its identity's lease was attempted. NotFound
// or an expired context rejects the bind instead; other extension errors are
// logged and left to lease renewal.
type OnNewListener func(listener net.Listener, established uint)

// EstablishedCounter is implemented by SDK multi-listeners. A listener that
// cannot report router-confirmed terminators is never treated as ready.
type EstablishedCounter interface {
	GetEstablishedCount() uint
}

// errorEventSource is the SDK multi-listener hook for child bind failures.
type errorEventSource interface {
	SetErrorEventHandler(func(error))
}

// Manager owns the gateway's Ziti service identity and listener. The gateway is
// ready only while the current listener has an established terminator; every
// failure path closes the identity's context and clears state instead of
// keeping a listener that routers reject.
type Manager struct {
	mu              sync.RWMutex
	enrollMu        sync.Mutex
	zitiCtx         ziti.Context
	listener        net.Listener
	identityID      string
	mgmtClient      *zitimgmtclient.Client
	serviceType     zitimgmtv1.ServiceType
	renewalInterval time.Duration
	enrollTimeout   time.Duration
	bindTimeout     time.Duration

	listenerFactory ListenerFactory
	onNewListener   OnNewListener
}

// New validates the configuration. Call Start to enroll and bind, then Run to
// keep the identity leased and the listener established.
func New(
	client *zitimgmtclient.Client,
	serviceType zitimgmtv1.ServiceType,
	enrollTimeout time.Duration,
	renewalInterval time.Duration,
	bindTimeout time.Duration,
	listenerFactory ListenerFactory,
	onNewListener OnNewListener,
) (*Manager, error) {
	if client == nil {
		return nil, errors.New("ziti management client is required")
	}
	if serviceType == zitimgmtv1.ServiceType_SERVICE_TYPE_UNSPECIFIED {
		return nil, errors.New("service type is required")
	}
	if enrollTimeout <= 0 {
		return nil, errors.New("enroll timeout must be positive")
	}
	if renewalInterval <= 0 {
		return nil, errors.New("renewal interval must be positive")
	}
	if bindTimeout <= 0 {
		return nil, errors.New("bind timeout must be positive")
	}
	if listenerFactory == nil {
		return nil, errors.New("listener factory is required")
	}
	if onNewListener == nil {
		return nil, errors.New("on new listener callback is required")
	}

	return &Manager{
		mgmtClient:      client,
		serviceType:     serviceType,
		renewalInterval: renewalInterval,
		enrollTimeout:   enrollTimeout,
		bindTimeout:     bindTimeout,
		listenerFactory: listenerFactory,
		onNewListener:   onNewListener,
	}, nil
}

func (m *Manager) ZitiContext() ziti.Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.zitiCtx
}

// EstablishedListeners reports router-confirmed terminators of the current
// listener. It is zero while enrolling or binding and after terminators are lost.
func (m *Manager) EstablishedListeners() uint {
	m.mu.RLock()
	listener := m.listener
	m.mu.RUnlock()
	return establishedCount(listener)
}

// Start enrolls and binds, retrying with backoff until a listener is
// established or the enrollment timeout expires.
func (m *Manager) Start(ctx context.Context) error {
	return m.enroll(ctx)
}

// Run keeps the identity leased and the listener established until ctx ends.
// Lease renewal runs in its own goroutine, so a slow ziti-management cannot
// delay the watchdog. The manager re-enrolls when ziti-management no longer
// knows the current identity or the listener has had no established terminator
// for longer than the bind timeout, and keeps retrying until a listener is
// established: losing Ziti after startup reports not ready but never stops the
// gateway's TCP API.
func (m *Manager) Run(ctx context.Context) {
	leaseLost := make(chan string)
	renewalDone := make(chan struct{})
	go func() {
		defer close(renewalDone)
		m.renewLease(ctx, leaseLost)
	}()
	defer func() { <-renewalDone }()

	watchdog := time.NewTicker(watchdogInterval)
	defer watchdog.Stop()

	var unestablishedSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case identityID := <-leaseLost:
			// Ignore losses of identities that were already replaced.
			if identityID != m.identity() {
				continue
			}
			m.reEnroll(ctx, fmt.Sprintf("lease of identity %s not found", identityID))
			unestablishedSince = time.Time{}
		case <-watchdog.C:
			if m.EstablishedListeners() > 0 {
				unestablishedSince = time.Time{}
				continue
			}
			if unestablishedSince.IsZero() {
				unestablishedSince = time.Now()
				log.Printf("ziti service listener has no established terminators")
				continue
			}
			if elapsed := time.Since(unestablishedSince); elapsed >= m.bindTimeout {
				m.reEnroll(ctx, fmt.Sprintf("no established terminators for %s", elapsed.Round(time.Millisecond)))
				unestablishedSince = time.Time{}
			}
		}
	}
}

// renewLease extends the current identity's lease every renewal interval and
// sends the identity on lost when ziti-management reports it NotFound. Without
// an identity there is nothing to extend: an enrollment is in progress or the
// watchdog will start one.
func (m *Manager) renewLease(ctx context.Context, lost chan<- string) {
	ticker := time.NewTicker(m.renewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		identityID := m.identity()
		if identityID == "" {
			continue
		}
		err := m.extendLeaseWithRetry(ctx, identityID)
		switch {
		case err == nil || ctx.Err() != nil:
		case status.Code(err) == codes.NotFound:
			select {
			case lost <- identityID:
			case <-ctx.Done():
				return
			}
		default:
			log.Printf("failed to extend ziti lease, retrying at the next renewal: %v", err)
		}
	}
}

// reEnroll replaces the current identity, retrying until a listener is
// established or ctx ends. Each round is one enroll, bounded by the enrollment
// timeout; failed rounds back off along reEnrollRoundBackoffs. A failed attempt
// abandons at most one identity whose lease was never extended, which
// ziti-management expires after its lease TTL.
func (m *Manager) reEnroll(ctx context.Context, reason string) {
	log.Printf("re-enrolling ziti identity: %s", reason)
	for round := 1; ; round++ {
		err := m.enroll(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}

		delay := backoffDelay(reEnrollRoundBackoffs, round)
		log.Printf("ziti re-enrollment round %d failed, staying not ready and retrying in %s: %v", round, delay, err)
		if sleepWithContext(ctx, delay) != nil {
			return
		}
	}
}

func (m *Manager) enroll(ctx context.Context) error {
	m.enrollMu.Lock()
	defer m.enrollMu.Unlock()

	m.clear()

	enrollmentCtx, cancel := context.WithTimeout(ctx, m.enrollTimeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		err := m.enrollOnce(enrollmentCtx)
		if err == nil {
			return nil
		}
		if enrollmentCtx.Err() != nil {
			return fmt.Errorf("ziti service listener not established within %s: %w", m.enrollTimeout, err)
		}

		delay := backoffDelay(reEnrollBackoffs, attempt)
		log.Printf("ziti enrollment attempt %d failed, retrying in %s: %v", attempt, delay, err)
		if sleepWithContext(enrollmentCtx, delay) != nil {
			return fmt.Errorf("ziti service listener not established within %s: %w", m.enrollTimeout, err)
		}
	}
}

// enrollOnce requests a fresh identity and binds it. ziti-management starts the
// identity's lease on issue, so the lease is extended as soon as a terminator is
// established; NotFound means the identity is already gone and fails the
// attempt. Other extension errors are left to Run's lease renewal, which the
// configuration bounds to fire before the lease expires.
func (m *Manager) enrollOnce(ctx context.Context) error {
	var identityID string
	var identityJSON []byte
	if err := retryWithBackoff(ctx, "ziti enrollment", func(attemptCtx context.Context) error {
		var requestErr error
		identityID, identityJSON, requestErr = m.mgmtClient.RequestServiceIdentity(attemptCtx, m.serviceType)
		return requestErr
	}); err != nil {
		return err
	}

	zitiConfig := &ziti.Config{}
	if err := json.Unmarshal(identityJSON, zitiConfig); err != nil {
		return fmt.Errorf("failed to parse ziti identity: %w", err)
	}

	zitiCtx, err := newZitiContext(zitiConfig)
	if err != nil {
		return fmt.Errorf("failed to create ziti context: %w", err)
	}

	listener, err := m.listen(ctx, zitiCtx)
	if err != nil {
		zitiCtx.Close()
		return err
	}
	discard := func() {
		_ = listener.Close()
		zitiCtx.Close()
	}

	established := establishedCount(listener)
	if established < requiredEstablishedListeners {
		discard()
		return errNotEstablished
	}

	if err := m.extendLeaseWithRetry(ctx, identityID); err != nil {
		if ctx.Err() != nil || status.Code(err) == codes.NotFound {
			discard()
			return fmt.Errorf("failed to extend ziti lease after bind: %w", err)
		}
		log.Printf("failed to extend ziti lease after bind, renewal will retry: %v", err)
	}

	if source, ok := listener.(errorEventSource); ok {
		source.SetErrorEventHandler((&bindErrorLogger{}).log)
	}

	// Publish state before the hand-off so readiness and ZitiContext already
	// agree with the "listening" line the callback logs.
	m.mu.Lock()
	m.zitiCtx = zitiCtx
	m.listener = listener
	m.identityID = identityID
	m.mu.Unlock()

	m.onNewListener(listener, established)

	return nil
}

// listen bounds the whole bind, including SDK authentication and service
// lookup, by the bind timeout. A listener returned after the deadline is closed.
func (m *Manager) listen(ctx context.Context, zitiCtx ziti.Context) (net.Listener, error) {
	options := ziti.DefaultListenOptions()
	options.WaitForNEstablishedListeners = requiredEstablishedListeners
	// In SDK v1.6.0 ConnectTimeout is the WaitForN deadline and also the
	// MaxElapsedTime of the listener's bind-session (createSessionWithBackoff)
	// and re-authentication (EnsureAuthenticated) backoffs, here and on later
	// session refreshes. Router bind replies keep a fixed 5s timeout.
	options.ConnectTimeout = m.bindTimeout

	type result struct {
		listener net.Listener
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		listener, err := m.listenerFactory(zitiCtx, options)
		resultCh <- result{listener: listener, err: err}
	}()

	timer := time.NewTimer(m.bindTimeout)
	defer timer.Stop()

	var err error
	select {
	case r := <-resultCh:
		if r.err != nil {
			return nil, fmt.Errorf("failed to bind ziti service: %w", r.err)
		}
		if r.listener == nil {
			return nil, errors.New("failed to bind ziti service: listener is nil")
		}
		return r.listener, nil
	case <-timer.C:
		err = fmt.Errorf("ziti service bind did not complete within %s", m.bindTimeout)
	case <-ctx.Done():
		err = ctx.Err()
	}

	go func() {
		if r := <-resultCh; r.listener != nil {
			_ = r.listener.Close()
		}
	}()
	return nil, err
}

// clear closes the current listener and context so the gateway reports not
// ready until a new listener is established.
func (m *Manager) clear() {
	m.mu.Lock()
	zitiCtx, listener := m.zitiCtx, m.listener
	m.zitiCtx = nil
	m.listener = nil
	m.identityID = ""
	m.mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}
	if zitiCtx != nil {
		zitiCtx.Close()
	}
}

func (m *Manager) extendLeaseWithRetry(ctx context.Context, identityID string) error {
	if identityID == "" {
		return errIdentityMissing
	}
	var lastErr error
	for attempt := 0; attempt <= len(leaseRetryBackoffs); attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, leaseRetryBackoffs[attempt-1]); err != nil {
				return err
			}
		}
		lastErr = m.extendLease(ctx, identityID)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRetryableGrpcError(lastErr) {
			return lastErr
		}
	}

	return lastErr
}

// extendLease makes one ExtendIdentityLease attempt bounded by
// leaseExtendAttemptTimeout. The resulting DeadlineExceeded is not retryable: a
// hung ziti-management is unlikely to answer the next attempt, and the next
// renewal tick tries again.
func (m *Manager) extendLease(ctx context.Context, identityID string) error {
	attemptCtx, cancel := context.WithTimeout(ctx, leaseExtendAttemptTimeout)
	defer cancel()
	return m.mgmtClient.ExtendIdentityLease(attemptCtx, identityID)
}

func (m *Manager) identity() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.identityID
}

func establishedCount(listener net.Listener) uint {
	counter, ok := listener.(EstablishedCounter)
	if !ok {
		return 0
	}
	return counter.GetEstablishedCount()
}

// bindErrorLogger logs child bind failures reported after establishment, such
// as "identity not found by id". SDK errors carry router messages, not
// credentials; the identity JSON is never logged.
type bindErrorLogger struct {
	mu         sync.Mutex
	lastLogged time.Time
	suppressed int
}

func (l *bindErrorLogger) log(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.lastLogged.IsZero() && time.Since(l.lastLogged) < bindErrorLogInterval {
		l.suppressed++
		return
	}
	if l.suppressed > 0 {
		log.Printf("ziti service listener bind error (%d earlier errors suppressed): %v", l.suppressed, err)
	} else {
		log.Printf("ziti service listener bind error: %v", err)
	}
	l.lastLogged = time.Now()
	l.suppressed = 0
}

// backoffDelay returns the delay after the given 1-based attempt; the last
// entry repeats.
func backoffDelay(backoffs []time.Duration, attempt int) time.Duration {
	if attempt <= 0 || len(backoffs) == 0 {
		return 0
	}
	idx := attempt - 1
	if idx >= len(backoffs) {
		idx = len(backoffs) - 1
	}
	return backoffs[idx]
}

func retryWithBackoff(ctx context.Context, operationName string, fn func(context.Context) error) error {
	backoff := retryInitialBackoff
	attempt := 1
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if !isRetryableGrpcError(err) {
			return err
		}

		delay := backoff
		if delay > retryMaxBackoff {
			delay = retryMaxBackoff
		}

		log.Printf("%s failed (attempt %d), retrying in %s: %v", operationName, attempt, delay, err)

		if err := sleepWithContext(ctx, delay); err != nil {
			return err
		}

		backoff *= 2
		if backoff > retryMaxBackoff {
			backoff = retryMaxBackoff
		}
		attempt++
	}
}

func isRetryableGrpcError(err error) bool {
	statusErr, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusErr.Code() == codes.Unavailable || statusErr.Code() == codes.Unknown
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
