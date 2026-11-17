package main

import (
	"flag"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	galleryID := flag.Uint("gallery-id", 0, "gallery ID to inspect")
	all := flag.Bool("all", false, "inspect every gallery with an HTTP(S) source")
	serverURL := flag.String("server-url", "", "running API base URL (default uses configured port on localhost)")
	flag.Parse()
	if (*galleryID == 0 && !*all) || (*galleryID != 0 && *all) {
		return fmt.Errorf("usage: go run ./cmd/recover-source (-gallery-id <id> | -all)")
	}

	config.Load()
	logger.SetLevelFromString(config.Global.LogLevel)
	if *serverURL == "" {
		*serverURL = "http://localhost:" + config.Global.Port
	}
	database.Connect(config.Global.DatabaseDSN())
	sqlDB, err := database.DB.DB()
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	defer sqlDB.Close()
	apiClient := &http.Client{Timeout: 10 * time.Second}

	if *all {
		return auditAll(*serverURL, apiClient)
	}

	report, err := services.AuditGallerySource(uint(*galleryID))
	if err != nil {
		return fmt.Errorf("recover source: %w", err)
	}
	printAudit(report)
	if report.SourceRequeued {
		if err := enqueueSource(apiClient, *serverURL, report.SourceID); err != nil {
			return fmt.Errorf("request crawl for source %d (it remains queued for startup): %w", report.SourceID, err)
		}
		fmt.Printf("Live crawl requested for source %d.\n", report.SourceID)
	}
	return nil
}

func auditAll(serverURL string, apiClient *http.Client) error {
	batch, err := services.AuditAllGallerySources()
	if err != nil {
		return err
	}
	requested := 0
	requestFailures := 0
	for _, audit := range batch.Audits {
		printAudit(audit)
		if audit.SourceRequeued {
			if err := enqueueSource(apiClient, serverURL, audit.SourceID); err != nil {
				fmt.Fprintf(os.Stderr, "Gallery %d: crawl request for source %d failed (source remains queued for startup): %v\n",
					audit.GalleryID, audit.SourceID, err)
				requestFailures++
				continue
			}
			requested++
			fmt.Printf("Live crawl requested for source %d.\n", audit.SourceID)
		}
	}
	for _, failure := range batch.Failures {
		fmt.Fprintf(os.Stderr, "Gallery %d audit failed: %v\n", failure.GalleryID, failure.Err)
	}
	fmt.Printf("Audit complete: checked %d, crawl requests %d, skipped %d, audit failures %d, request failures %d.\n",
		len(batch.Audits), requested, batch.Skipped, len(batch.Failures), requestFailures)
	if len(batch.Failures)+requestFailures > 0 {
		return fmt.Errorf("%d gallery operations failed", len(batch.Failures)+requestFailures)
	}
	return nil
}

func enqueueSource(client *http.Client, serverURL string, sourceID uint) error {
	baseURL, err := url.Parse(serverURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return fmt.Errorf("invalid API base URL")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + fmt.Sprintf("/sources/%d/crawl", sourceID)
	req, err := http.NewRequest(http.MethodPost, baseURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build crawl request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send crawl request: %w", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("read crawl response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func printAudit(report *services.GallerySourceAudit) {
	fmt.Printf("Gallery %d (%s): saved photos %d; photos in source post %d\n",
		report.GalleryID, report.GalleryName, report.GalleryPhotos, report.SourcePhotos)
	if report.SourceRequeued {
		fmt.Printf("Source %d marked queued; requesting a live crawl.\n", report.SourceID)
	} else if report.SourcePending {
		fmt.Printf("Source %d is already queued or crawling.\n", report.SourceID)
	} else {
		fmt.Printf("Counts match; source %d was not requeued.\n", report.SourceID)
	}
}
