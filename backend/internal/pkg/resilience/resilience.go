package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	name             string
	failureThreshold int
	openAfter        time.Duration
	now              func() time.Time

	mu               sync.Mutex
	state            CircuitState
	failures         int
	openedAt         time.Time
	halfOpenInFlight bool
}

type HTTPPolicy struct {
	Name               string
	MaxAttempts        int
	InitialBackoff     time.Duration
	MaxBackoff         time.Duration
	FailureThreshold   int
	OpenAfter          time.Duration
	RetryUnsafeMethods bool
	Sleep              func(context.Context, time.Duration) error
}

type OperationPolicy struct {
	Name             string
	MaxAttempts      int
	InitialBackoff   time.Duration
	MaxBackoff       time.Duration
	FailureThreshold int
	OpenAfter        time.Duration
	Sleep            func(context.Context, time.Duration) error
}

type ResilientRoundTripper struct {
	base    http.RoundTripper
	policy  HTTPPolicy
	breaker *CircuitBreaker
}

var breakers struct {
	sync.Mutex
	byName map[string]*CircuitBreaker
}

func init() {
	breakers.byName = make(map[string]*CircuitBreaker)
}

func DefaultHTTPPolicy(name string) HTTPPolicy {
	return HTTPPolicy{
		Name:             name,
		MaxAttempts:      3,
		InitialBackoff:   200 * time.Millisecond,
		MaxBackoff:       2 * time.Second,
		FailureThreshold: 5,
		OpenAfter:        30 * time.Second,
	}
}

func DefaultOperationPolicy(name string) OperationPolicy {
	return OperationPolicy{
		Name:             name,
		MaxAttempts:      2,
		InitialBackoff:   500 * time.Millisecond,
		MaxBackoff:       2 * time.Second,
		FailureThreshold: 3,
		OpenAfter:        60 * time.Second,
	}
}

func NewHTTPClient(name string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewRoundTripper(http.DefaultTransport, DefaultHTTPPolicy(name)),
	}
}

func Run(ctx context.Context, policy OperationPolicy, operation func(context.Context) error) error {
	policy = normalizeOperationPolicy(policy)
	breaker := breakerForOperation(policy)
	if err := breaker.Allow(); err != nil {
		return fmt.Errorf("%s: %w", policy.Name, err)
	}

	var lastErr error
	backoff := policy.InitialBackoff
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		err := operation(ctx)
		if err == nil {
			breaker.Record(true)
			return nil
		}
		lastErr = err
		retryable := RetryableError(err)
		if !retryable {
			breaker.Record(true)
			return err
		}
		if attempt == policy.MaxAttempts {
			breaker.Record(false)
			return err
		}
		if err := policy.Sleep(ctx, backoff); err != nil {
			breaker.Record(false)
			return err
		}
		backoff = nextBackoff(backoff, policy.MaxBackoff)
	}

	breaker.Record(false)
	return lastErr
}

func NewRoundTripper(base http.RoundTripper, policy HTTPPolicy) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	policy = normalizeHTTPPolicy(policy)
	return &ResilientRoundTripper{
		base:    base,
		policy:  policy,
		breaker: breakerFor(policy),
	}
}

func (rt *ResilientRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt == nil {
		return nil, errors.New("resilient round tripper is nil")
	}
	if err := rt.breaker.Allow(); err != nil {
		return nil, fmt.Errorf("%s: %w", rt.policy.Name, err)
	}

	var lastErr error
	var lastResp *http.Response
	backoff := rt.policy.InitialBackoff
	failed := false

	for attempt := 1; attempt <= rt.policy.MaxAttempts; attempt++ {
		attemptReq, err := cloneRequestForAttempt(req, attempt)
		if err != nil {
			rt.breaker.Record(false)
			return nil, err
		}

		resp, err := rt.base.RoundTrip(attemptReq)
		retry := retryableHTTPFailure(attemptReq, resp, err, rt.policy)
		failure := failedHTTPAttempt(resp, err)
		if !retry {
			rt.breaker.Record(!failure)
			return resp, err
		}

		failed = failed || failure
		lastErr = err
		lastResp = resp
		if attempt == rt.policy.MaxAttempts {
			break
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err := rt.policy.Sleep(req.Context(), backoff); err != nil {
			rt.breaker.Record(false)
			return nil, err
		}
		backoff = nextBackoff(backoff, rt.policy.MaxBackoff)
	}

	rt.breaker.Record(!failed)
	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func NewCircuitBreaker(name string, failureThreshold int, openAfter time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 1
	}
	if openAfter <= 0 {
		openAfter = 30 * time.Second
	}
	return &CircuitBreaker{
		name:             name,
		failureThreshold: failureThreshold,
		openAfter:        openAfter,
		now:              time.Now,
		state:            CircuitClosed,
	}
}

func (b *CircuitBreaker) Allow() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == CircuitClosed {
		return nil
	}
	if b.state == CircuitOpen {
		if b.now().Sub(b.openedAt) >= b.openAfter {
			b.state = CircuitHalfOpen
			b.halfOpenInFlight = true
			return nil
		}
		return ErrCircuitOpen
	}
	if b.halfOpenInFlight {
		return ErrCircuitOpen
	}
	b.halfOpenInFlight = true
	return nil
}

func (b *CircuitBreaker) Record(success bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case CircuitHalfOpen:
		b.halfOpenInFlight = false
		if success {
			b.state = CircuitClosed
			b.failures = 0
			b.openedAt = time.Time{}
			return
		}
		b.open()
	case CircuitOpen:
		if success {
			b.state = CircuitClosed
			b.failures = 0
			b.openedAt = time.Time{}
		}
	default:
		if success {
			b.failures = 0
			return
		}
		b.failures++
		if b.failures >= b.failureThreshold {
			b.open()
		}
	}
}

func (b *CircuitBreaker) State() CircuitState {
	if b == nil {
		return CircuitClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *CircuitBreaker) open() {
	b.state = CircuitOpen
	b.openedAt = b.now()
	b.halfOpenInFlight = false
}

func normalizeHTTPPolicy(policy HTTPPolicy) HTTPPolicy {
	if policy.Name == "" {
		policy.Name = "external"
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.InitialBackoff < 0 {
		policy.InitialBackoff = 0
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		policy.MaxBackoff = policy.InitialBackoff
	}
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = 1
	}
	if policy.OpenAfter <= 0 {
		policy.OpenAfter = 30 * time.Second
	}
	if policy.Sleep == nil {
		policy.Sleep = sleepContext
	}
	return policy
}

func normalizeOperationPolicy(policy OperationPolicy) OperationPolicy {
	if policy.Name == "" {
		policy.Name = "external-operation"
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.InitialBackoff < 0 {
		policy.InitialBackoff = 0
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		policy.MaxBackoff = policy.InitialBackoff
	}
	if policy.FailureThreshold <= 0 {
		policy.FailureThreshold = 1
	}
	if policy.OpenAfter <= 0 {
		policy.OpenAfter = 30 * time.Second
	}
	if policy.Sleep == nil {
		policy.Sleep = sleepContext
	}
	return policy
}

func breakerFor(policy HTTPPolicy) *CircuitBreaker {
	breakers.Lock()
	defer breakers.Unlock()

	if breaker, ok := breakers.byName[policy.Name]; ok {
		return breaker
	}
	breaker := NewCircuitBreaker(policy.Name, policy.FailureThreshold, policy.OpenAfter)
	breakers.byName[policy.Name] = breaker
	return breaker
}

func breakerForOperation(policy OperationPolicy) *CircuitBreaker {
	breakers.Lock()
	defer breakers.Unlock()

	if breaker, ok := breakers.byName[policy.Name]; ok {
		return breaker
	}
	breaker := NewCircuitBreaker(policy.Name, policy.FailureThreshold, policy.OpenAfter)
	breakers.byName[policy.Name] = breaker
	return breaker
}

func cloneRequestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 1 {
		return req, nil
	}
	clone := req.Clone(req.Context())
	if req.Body == nil {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body cannot be replayed for retry")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

func retryableHTTPFailure(req *http.Request, resp *http.Response, err error, policy HTTPPolicy) bool {
	if !methodAllowsRetry(req.Method, policy) {
		return false
	}
	return failedHTTPAttempt(resp, err)
}

func failedHTTPAttempt(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return true
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError
}

func methodAllowsRetry(method string, policy HTTPPolicy) bool {
	if policy.RetryUnsafeMethods {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func RetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, ErrCircuitOpen) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	message := strings.ToLower(err.Error())
	retryableFragments := []string{
		"timeout",
		"timed out",
		"temporary",
		"connection refused",
		"connection reset",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"too many requests",
		"returned 429",
		"returned 500",
		"returned 502",
		"returned 503",
		"returned 504",
		"status 429",
		"status 500",
		"status 502",
		"status 503",
		"status 504",
	}
	for _, fragment := range retryableFragments {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func nextBackoff(current, max time.Duration) time.Duration {
	if current <= 0 {
		return 0
	}
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
