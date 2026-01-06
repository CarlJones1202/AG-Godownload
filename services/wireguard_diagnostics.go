package services

import (
	"fmt"
	"gallery_api/logger"
	"io"
)

// TestWireGuardConnection tests if the WireGuard tunnel is actually routing traffic
// AND tests the MetArt API endpoint provided by the user
func TestWireGuardConnection() error {
	logger.Infof("[WireGuard Test] Starting diagnostics...")

	// 1. Verify VPN Connection and API Access
	// The user provided this specific API URL which bypasses the SSR shell
	apiURL := "https://www.metart.com/api/search-results?searchPhrase=presenting%20libby&page=1&pageSize=30&sortBy=latest-gallery"

	logger.Infof("[WireGuard Test] calling MetArt API: %s", apiURL)

	// Use the VPN client
	wgClient := GetHTTPClient(apiURL)

	resp, err := wgClient.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	logger.Infof("[WireGuard Test] Status: %d", resp.StatusCode)

	// Print the JSON body (truncated if too long, but long enough to see schema)
	jsonStr := string(bodyBytes)
	if len(jsonStr) > 2000 {
		logger.Infof("[WireGuard Test] JSON Response (first 2000 chars): %s", jsonStr[:2000])
	} else {
		logger.Infof("[WireGuard Test] JSON Response: %s", jsonStr)
	}

	logger.Infof("[WireGuard Test] Diagnostics complete.")
	return nil
}
