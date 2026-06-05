package resilience

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResilientRoundTripperRetriesRetryableStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewRoundTripper(server.Client().Transport, HTTPPolicy{
			Name:             "test-retry-status",
			MaxAttempts:      2,
			FailureThreshold: 3,
			OpenAfter:        time.Second,
			Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
		}),
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	breaker := NewCircuitBreaker("test", 2, time.Minute)

	require.NoError(t, breaker.Allow())
	breaker.Record(false)
	require.Equal(t, CircuitClosed, breaker.State())

	require.NoError(t, breaker.Allow())
	breaker.Record(false)
	require.Equal(t, CircuitOpen, breaker.State())
	require.ErrorIs(t, breaker.Allow(), ErrCircuitOpen)
}

func TestCircuitBreakerHalfOpenClosesOnSuccess(t *testing.T) {
	now := time.Now()
	breaker := NewCircuitBreaker("test", 1, time.Second)
	breaker.now = func() time.Time { return now }

	require.NoError(t, breaker.Allow())
	breaker.Record(false)
	require.Equal(t, CircuitOpen, breaker.State())

	now = now.Add(time.Second)
	require.NoError(t, breaker.Allow())
	require.Equal(t, CircuitHalfOpen, breaker.State())

	breaker.Record(true)
	require.Equal(t, CircuitClosed, breaker.State())
}

func TestResilientRoundTripperDoesNotRetryUnsafeMethodsByDefault(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewRoundTripper(server.Client().Transport, HTTPPolicy{
			Name:             "test-unreplayable",
			MaxAttempts:      2,
			FailureThreshold: 3,
			OpenAfter:        time.Second,
			Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
		}),
	}

	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewBufferString("payload"))
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.Equal(t, 1, attempts)
}

func TestResilientRoundTripperCountsUnretriedServerErrorsAsFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewRoundTripper(server.Client().Transport, HTTPPolicy{
			Name:             "test-post-breaker",
			MaxAttempts:      2,
			FailureThreshold: 2,
			OpenAfter:        time.Minute,
			Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
		}),
	}

	for range 2 {
		resp, err := client.Post(server.URL, "text/plain", bytes.NewBufferString("payload"))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	resp, err := client.Post(server.URL, "text/plain", bytes.NewBufferString("payload"))
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.ErrorIs(t, err, ErrCircuitOpen)
	require.Equal(t, 2, attempts)
}

func TestResilientRoundTripperRejectsUnreplayableRetryBodyWhenUnsafeOptedIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewRoundTripper(server.Client().Transport, HTTPPolicy{
			Name:               "test-unreplayable",
			MaxAttempts:        2,
			FailureThreshold:   3,
			OpenAfter:          time.Second,
			RetryUnsafeMethods: true,
			Sleep:              func(_ context.Context, _ time.Duration) error { return nil },
		}),
	}

	req, err := http.NewRequest(http.MethodPost, server.URL, io.NopCloser(strings.NewReader("payload")))
	require.NoError(t, err)

	resp, err := client.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be replayed")
}

func TestRunRetriesRetryableOperationError(t *testing.T) {
	attempts := 0
	err := Run(t.Context(), OperationPolicy{
		Name:             "test-operation-retry",
		MaxAttempts:      2,
		FailureThreshold: 3,
		OpenAfter:        time.Second,
		Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
	}, func(_ context.Context) error {
		attempts++
		if attempts == 1 {
			return errors.New("gateway timeout")
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func TestRunDoesNotRetryNonRetryableOperationError(t *testing.T) {
	attempts := 0
	err := Run(t.Context(), OperationPolicy{
		Name:             "test-operation-no-retry",
		MaxAttempts:      2,
		FailureThreshold: 3,
		OpenAfter:        time.Second,
		Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
	}, func(_ context.Context) error {
		attempts++
		return errors.New("login expired")
	})

	require.Error(t, err)
	require.Equal(t, 1, attempts)
}

func TestRunDoesNotOpenCircuitForNonRetryableOperationErrors(t *testing.T) {
	attempts := 0
	policy := OperationPolicy{
		Name:             "test-operation-user-error",
		MaxAttempts:      2,
		FailureThreshold: 2,
		OpenAfter:        time.Minute,
		Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
	}

	for range 3 {
		err := Run(t.Context(), policy, func(_ context.Context) error {
			attempts++
			return errors.New("invalid credentials")
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCircuitOpen)
	}

	require.Equal(t, 3, attempts)
}

func TestRunOpensCircuitForRetryableOperationErrors(t *testing.T) {
	attempts := 0
	policy := OperationPolicy{
		Name:             "test-operation-breaker",
		MaxAttempts:      1,
		FailureThreshold: 2,
		OpenAfter:        time.Minute,
		Sleep:            func(_ context.Context, _ time.Duration) error { return nil },
	}

	for range 2 {
		err := Run(t.Context(), policy, func(_ context.Context) error {
			attempts++
			return errors.New("gateway timeout")
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrCircuitOpen)
	}

	err := Run(t.Context(), policy, func(_ context.Context) error {
		attempts++
		return nil
	})
	require.ErrorIs(t, err, ErrCircuitOpen)
	require.Equal(t, 2, attempts)
}
