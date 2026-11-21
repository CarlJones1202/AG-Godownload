package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gallery_api/config"
)

func TestIMXHostDetection(t *testing.T) {
	for _, host := range []string{"imx.to", "image.imx.to", "i02.imx.to"} {
		if !isIMXHost(host) {
			t.Errorf("isIMXHost(%q) = false, want true", host)
		}
	}
	if isIMXHost("notimx.to") {
		t.Error("isIMXHost accepted a non-IMX domain")
	}
}

func TestAcquireIMXRequestLimitsConcurrency(t *testing.T) {
	first, err := acquireIMXRequest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireIMXRequest(context.Background())
	if err != nil {
		first()
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	third, err := acquireIMXRequest(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		if err == nil {
			third()
		}
		first()
		second()
		t.Fatalf("third concurrent acquire error = %v, want deadline exceeded", err)
	}

	first()
	third, err = acquireIMXRequest(context.Background())
	if err != nil {
		second()
		t.Fatalf("acquire after a slot is released: %v", err)
	}
	second()
	third()
}

func TestDoRequestWithRetryUsesExtendedRetriesForIMX(t *testing.T) {
	oldClient, oldTransport := globalClient, globalTransport
	oldRetries := config.Global.HTTPMaxRetries
	globalTransport = nil
	config.Global.HTTPMaxRetries = 1
	var requestCount atomic.Int32
	globalClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		count := requestCount.Add(1)
		status := http.StatusServiceUnavailable
		if count == 5 {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Retry-After": []string{"0"}},
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})}
	defer func() {
		globalClient, globalTransport = oldClient, oldTransport
		config.Global.HTTPMaxRetries = oldRetries
	}()

	req, err := http.NewRequest(http.MethodGet, "https://image.imx.to/image.jpg", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DoRequestWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("DoRequestWithRetry() error = %v", err)
	}
	resp.Body.Close()
	if got := requestCount.Load(); got != 5 {
		t.Fatalf("request count = %d, want 5", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, time.October, 22, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		retry      int
		retryAfter string
		want       time.Duration
	}{
		{name: "first retry backoff", retry: 1, want: time.Second},
		{name: "exponential backoff", retry: 3, want: 4 * time.Second},
		{name: "backoff cap", retry: 10, want: maxRetryBackoff},
		{name: "retry-after seconds", retry: 1, retryAfter: "5", want: 5 * time.Second},
		{name: "retry-after cap", retry: 1, retryAfter: "120", want: maxRetryAfter},
		{name: "retry-after date", retry: 1, retryAfter: now.Add(7 * time.Second).Format(http.TimeFormat), want: 7 * time.Second},
		{name: "past retry-after date", retry: 1, retryAfter: now.Add(-time.Second).Format(http.TimeFormat), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryDelay(tt.retry, tt.retryAfter, now); got != tt.want {
				t.Errorf("retryDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoRequestWithRetryRetriesServiceUnavailable(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldClient, oldTransport := globalClient, globalTransport
	oldRetries := config.Global.HTTPMaxRetries
	globalClient = server.Client()
	globalTransport = nil
	config.Global.HTTPMaxRetries = 1
	defer func() {
		globalClient, globalTransport = oldClient, oldTransport
		config.Global.HTTPMaxRetries = oldRetries
	}()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DoRequestWithRetry(context.Background(), req)
	if err != nil {
		t.Fatalf("DoRequestWithRetry() error = %v", err)
	}
	resp.Body.Close()

	if got := requestCount.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
}

func TestDoRequestWithRetryReportsExhaustedRetries(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	oldClient, oldTransport := globalClient, globalTransport
	oldRetries := config.Global.HTTPMaxRetries
	globalClient = server.Client()
	globalTransport = nil
	config.Global.HTTPMaxRetries = 1
	defer func() {
		globalClient, globalTransport = oldClient, oldTransport
		config.Global.HTTPMaxRetries = oldRetries
	}()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DoRequestWithRetry(context.Background(), req)
	if err == nil {
		t.Fatal("DoRequestWithRetry() error = nil, want exhausted-retries error")
	}

	var retryErr *requestRetryError
	if !errors.As(err, &retryErr) {
		t.Fatalf("DoRequestWithRetry() error type = %T, want *requestRetryError", err)
	}
	if got, want := err.Error(), fmt.Sprintf("request failed after %d retries: server returned status %d", 1, http.StatusServiceUnavailable); got != want {
		t.Errorf("DoRequestWithRetry() error = %q, want %q", got, want)
	}
	if got := requestCount.Load(); got != 2 {
		t.Errorf("server received %d requests, want 2", got)
	}
	if !isRequestRetryExhausted(err) {
		t.Error("isRequestRetryExhausted() = false, want true")
	}
}
