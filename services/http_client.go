package services

import (
	"context"
	"fmt"
	"gallery_api/config"
	"gallery_api/logger"
	"math"
	"net"
	"net/http"
	"time"
)

// HTTPClientConfig holds configuration for the HTTP client
type HTTPClientConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	ConnectTimeout      time.Duration
	RequestTimeout      time.Duration
	MaxRetries          int
}

// DefaultHTTPClientConfig returns sensible defaults from global config
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: config.Global.MaxConcurrentDownloads + 5, // slightly more than concurrent downloads
		IdleConnTimeout:     90 * time.Second,
		ConnectTimeout:      config.Global.HTTPConnectTimeout,
		RequestTimeout:      config.Global.HTTPRequestTimeout,
		MaxRetries:          config.Global.HTTPMaxRetries,
	}
}

var (
	globalClient    *http.Client
	globalTransport *http.Transport
)

// InitHTTPClient initializes the global HTTP client with connection pooling
func InitHTTPClient(config HTTPClientConfig) {
	globalTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	globalClient = &http.Client{
		Transport: globalTransport,
		Timeout:   config.RequestTimeout,
	}

	logger.Info("HTTP Client initialized with connection pooling")
}

// GetGlobalTransport returns the configured transport for use with other clients (like Colly)
func GetGlobalTransport() *http.Transport {
	if globalTransport == nil {
		InitHTTPClient(DefaultHTTPClientConfig())
	}
	return globalTransport
}

// GetHTTPClient returns the global configured client
func GetConfiguredHTTPClient() *http.Client {
	if globalClient == nil {
		InitHTTPClient(DefaultHTTPClientConfig())
	}
	return globalClient
}

// Check if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
}

// DoRequestWithRetry executes an HTTP request with exponential backoff retry logic
func DoRequestWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	client := GetConfiguredHTTPClient()
	config := DefaultHTTPClientConfig() // In a real app, this might come from global config

	var lastErr error

	for i := 0; i <= config.MaxRetries; i++ {
		// specific check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if i > 0 {
			// Exponential backoff: 2^i * 100ms, capped to a sensible maximum
			backoff := time.Duration(math.Pow(2, float64(i))) * 100 * time.Millisecond
			maxBackoff := 10 * time.Second
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			// Log the full URL for easier debugging
			logger.Debugf("Retrying request to %s in %v (attempt %d)", req.URL.String(), backoff, i+1)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Create a shallow copy of the request with the context
		reqWithCtx := req.WithContext(ctx)

		// Ensure User-Agent is set
		if reqWithCtx.Header.Get("User-Agent") == "" {
			reqWithCtx.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		}

		resp, err := client.Do(reqWithCtx)
		if err != nil {
			lastErr = err
			if isRetryableError(err) {
				continue
			}
			return nil, err
		}

		// Check for retryable status codes (5xx, 429)
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned status %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", config.MaxRetries, lastErr)
}
