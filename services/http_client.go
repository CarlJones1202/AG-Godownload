package services

import (
	"context"
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/logger"
	"net"
	"net/http"
	urlpkg "net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxRetryBackoff = 30 * time.Second
	maxRetryAfter   = time.Minute
)

type requestRetryError struct {
	retries int
	err     error
}

func (e *requestRetryError) Error() string {
	return fmt.Sprintf("request failed after %d retries: %v", e.retries, e.err)
}

func (e *requestRetryError) Unwrap() error {
	return e.err
}

func isRequestRetryExhausted(err error) bool {
	var retryErr *requestRetryError
	return errors.As(err, &retryErr)
}

func retryDelay(retry int, retryAfter string, now time.Time) time.Duration {
	backoff := time.Second
	for i := 1; i < retry && backoff < maxRetryBackoff; i++ {
		backoff *= 2
	}
	if backoff > maxRetryBackoff {
		backoff = maxRetryBackoff
	}

	retryAfter = strings.TrimSpace(retryAfter)
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		delay := time.Duration(seconds) * time.Second
		return delay
	}
	if retryAt, err := http.ParseTime(retryAfter); err == nil {
		delay := retryAt.Sub(now)
		if delay < 0 {
			return 0
		}
		if delay > maxRetryAfter {
			return maxRetryAfter
		}
		return delay
	}
	return backoff
}

// cookieCache caches session cookies per domain (e.g. vipr.im) so we only need
// to visit the hosting page once per crawl session to obtain them.
var cookieCache sync.Map

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
	var nextRetryDelay time.Duration
	hasNextRetryDelay := false

	for i := 0; i <= config.MaxRetries; i++ {
		// specific check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if i > 0 {
			backoff := retryDelay(i, "", time.Now())
			if hasNextRetryDelay {
				backoff = nextRetryDelay
				hasNextRetryDelay = false
			}
			logger.Debugf("Retrying request to %s in %v (attempt %d)", req.URL.String(), backoff, i+1)

			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
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
			if i < config.MaxRetries {
				nextRetryDelay = retryDelay(i+1, resp.Header.Get("Retry-After"), time.Now())
				hasNextRetryDelay = true
			}
			continue
		}

		return resp, nil
	}

	return nil, &requestRetryError{retries: config.MaxRetries, err: lastErr}
}

// GetOrFetchCookies returns session cookies for the given page URL. It caches
// cookies per domain so subsequent calls for the same domain return cached
// values without an additional HTTP round-trip.
func GetOrFetchCookies(pageURL string) ([]*http.Cookie, error) {
	u, err := urlpkg.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	domain := u.Hostname()

	// Return cached cookies if available
	if val, ok := cookieCache.Load(domain); ok {
		return val.([]*http.Cookie), nil
	}

	logger.Debugf("Fetching session cookies from %s", pageURL)
	gotCookies, err := visitPageForCookies(pageURL)
	if err != nil {
		logger.Warnf("Failed to visit %s for cookies: %v", pageURL, err)
		return nil, nil // fail open — caller falls back to regular DownloadImage
	}
	if len(gotCookies) > 0 {
		cookieCache.Store(domain, gotCookies)
		logger.Debugf("Cached %d cookies for domain %s (from %s)", len(gotCookies), domain, pageURL)
	}

	return gotCookies, nil
}

// ClearCookieCache clears the cached cookies (e.g. at the start of a new crawl).
// Entries are removed in place so in-flight callers holding a reference to the
// previous map instance are not affected by a wholesale replacement.
func ClearCookieCache() {
	cookieCache.Range(func(k, v any) bool {
		cookieCache.Delete(k)
		return true
	})
}
