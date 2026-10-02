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
	reEnrollBackoffs   = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
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
// established and the identity lease has been extended.
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

// Run renews the identity lease and re-enrolls when the lease is gone or the
// listener has had no established terminator for longer than the bind timeout.
// It returns an error, with state cleared, when re-enrollment fails within the
// enrollment timeout, and nil when ctx ends.
func (m *Manager) Run(ctx context.Context) error {
	renewal := time.NewTicker(m.renewalInterval)
	defer renewal.Stop()
	watchdog := time.NewTicker(watchdogInterval)
	defer watchdog.Stop()

	var unestablishedSince time.Time
	reEnroll := func(reason string) error {
		log.Printf("re-enrolling ziti identity: %s", reason)
		if err := m.enroll(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("failed to re-enroll ziti identity: %w", err)
		}
		unestablishedSince = time.Time{}
		renewal.Reset(m.renewalInterval)
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-renewal.C:
			err := m.extendLeaseWithRetry(ctx, m.identity())
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, errIdentityMissing) || status.Code(err) == codes.NotFound {
				if err := reEnroll(fmt.Sprintf("lease not found: %v", err)); err != nil {
					return err
				}
				continue
			}
			log.Printf("failed to extend ziti lease: %v", err)
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
				if err := reEnroll(fmt.Sprintf("no established terminators for %s", elapsed.Round(time.Millisecond))); err != nil {
					return err
				}
			}
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

		delay := reEnrollDelay(attempt)
		log.Printf("ziti enrollment attempt %d failed, retrying in %s: %v", attempt, delay, err)
		if sleepWithContext(enrollmentCtx, delay) != nil {
			return fmt.Errorf("ziti service listener not established within %s: %w", m.enrollTimeout, err)
		}
	}
}

// enrollOnce requests a fresh identity and binds it. ziti-management starts the
// identity's lease on issue, so the lease is extended as soon as a terminator is
// established; NotFound means the identity is already gone and fails the
// attempt. Other extension errors are left to Run's renewal ticker, which the
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

	m.onNewListener(listener, established)

	m.mu.Lock()
	m.zitiCtx = zitiCtx
	m.listener = listener
	m.identityID = identityID
	m.mu.Unlock()

	return nil
}

// listen bounds the whole bind, including SDK authentication and service
// lookup, by the bind timeout. A listener returned after the deadline is closed.
func (m *Manager) listen(ctx context.Context, zitiCtx ziti.Context) (net.Listener, error) {
	options := ziti.DefaultListenOptions()
	options.WaitForNEstablishedListeners = requiredEstablishedListeners
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
		lastErr = m.mgmtClient.ExtendIdentityLease(ctx, identityID)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if status.Code(lastErr) == codes.NotFound {
			return lastErr
		}
		if !isRetryableGrpcError(lastErr) {
			return lastErr
		}
	}

	return lastErr
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

func reEnrollDelay(attempt int) time.Duration {
	if attempt <= 0 || len(reEnrollBackoffs) == 0 {
		return 0
	}
	idx := attempt - 1
	if idx >= len(reEnrollBackoffs) {
		idx = len(reEnrollBackoffs) - 1
	}
	return reEnrollBackoffs[idx]
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
