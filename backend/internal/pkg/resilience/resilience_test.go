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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			Sleep:            func(ctx context.Context, delay time.Duration) error { return nil },
		}),
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			Sleep:            func(ctx context.Context, delay time.Duration) error { return nil },
		}),
	}

	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewBufferString("payload"))
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.Equal(t, 1, attempts)
}

func TestResilientRoundTripperRejectsUnreplayableRetryBodyWhenUnsafeOptedIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			Sleep:              func(ctx context.Context, delay time.Duration) error { return nil },
		}),
	}

	req, err := http.NewRequest(http.MethodPost, server.URL, io.NopCloser(strings.NewReader("payload")))
	require.NoError(t, err)

	_, err = client.Do(req)
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
		Sleep:            func(ctx context.Context, delay time.Duration) error { return nil },
	}, func(ctx context.Context) error {
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
		Sleep:            func(ctx context.Context, delay time.Duration) error { return nil },
	}, func(ctx context.Context) error {
		attempts++
		return errors.New("login expired")
	})

	require.Error(t, err)
	require.Equal(t, 1, attempts)
}
