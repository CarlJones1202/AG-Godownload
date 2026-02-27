package services

import (
	"context"
	"fmt"
	"gallery_api/logger"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/botanica-consulting/wiredialer"
)

var (
	wgDialer     *wiredialer.WireDialer
	wgDialerOnce sync.Once
	wgDialerErr  error
)

// GetWireGuardDialer returns a singleton WireGuard dialer if configured
func GetWireGuardDialer() (*wiredialer.WireDialer, error) {
	wgDialerOnce.Do(func() {
		confPath := os.Getenv("WIREGUARD_CONF")
		if confPath == "" {
			// Check default location
			confPath = "wireguard.conf"
		}

		// Check if file exists
		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			logger.Info("WireGuard config not found, scraper requests will use direct connection")
			wgDialerErr = fmt.Errorf("wireguard config not found at %s", confPath)
			return
		}

		logger.Infof("Loading WireGuard configuration from: %s", confPath)

		// Create the WireGuard dialer from file
		dialer, err := wiredialer.NewDialerFromFile(confPath)
		if err != nil {
			wgDialerErr = fmt.Errorf("failed to create WireGuard dialer: %w", err)
			logger.Errorf("Failed to create WireGuard dialer: %v", err)
			return
		}

		wgDialer = dialer
		logger.Info("WireGuard tunnel established successfully")
	})

	return wgDialer, wgDialerErr
}

// ShouldUseWireGuard determines if a URL should be routed through WireGuard
func ShouldUseWireGuard(targetURL string) bool {
	// List of domains that should use WireGuard tunnel
	blockedDomains := []string{
		"metart.com",
		"playboy.com",
		"femjoy.com",
		"met-art.com",
		"sexart.com",
		"thelifeerotic.com",
		"eternaldesire.com",
	}

	lowerURL := strings.ToLower(targetURL)
	for _, domain := range blockedDomains {
		if strings.Contains(lowerURL, domain) {
			return true
		}
	}
	return false
}

// GetHTTPClient returns an HTTP client, optionally using WireGuard for specific URLs
func GetHTTPClient(targetURL string) *http.Client {
	// Temporary bypass for testing
	if os.Getenv("BYPASS_VPN") == "true" {
		logger.Infof("[VPN] BYPASS_VPN=true, using direct connection for: %s", targetURL)
		return http.DefaultClient
	}

	if !ShouldUseWireGuard(targetURL) {
		logger.Debugf("[VPN] URL not in blocked list, using direct connection: %s", targetURL)
		return http.DefaultClient
	}

	logger.Infof("[VPN] URL requires WireGuard tunnel: %s", targetURL)

	dialer, err := GetWireGuardDialer()
	if err != nil {
		logger.Errorf("[VPN] WireGuard not available, falling back to direct connection: %v", err)
		logger.Errorf("[VPN] THIS WILL LIKELY FAIL DUE TO AGE VERIFICATION!")
		return http.DefaultClient
	}

	logger.Infof("[VPN] ✓ Using WireGuard tunnel for: %s", targetURL)

	// CRITICAL: We must also route DNS through the tunnel to avoid DNS leaks
	// The default resolver uses the system DNS which reveals our real location
	return &http.Client{
		Transport: &http.Transport{
			// Force IPv4 only - this prevents IPv6 leaks which are common causes of detection
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Force "tcp4" to ensure we don't accidentally use system IPv6
				return dialer.DialContext(ctx, "tcp4", addr)
			},
			DialTLSContext: nil, // Use DialContext for TLS too
		},
		// Disable automatic redirects to ensure all requests go through our custom transport
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
