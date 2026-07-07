package main

import (
	"encoding/json"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
	"os"
)

func main() {
	// Determine mode: purge (default) or recheck
	mode := "purge"
	providerFilter := ""
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--recheck":
			mode = "recheck"
		case "--provider":
			if i+1 < len(args) {
				i++
				providerFilter = args[i]
			}
		case "--help", "-h":
			fmt.Println("Usage: purge_placeholders [--recheck [--provider <name>]] [--help]")
			fmt.Println("  Default mode: scan filesystem and remove placeholder images")
			fmt.Println("  --recheck:   re-check DB images from given provider and re-download if placeholder")
			fmt.Println("  --provider:  provider filter for recheck (imagetwist, acidimg, all, etc.)")
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	// Load Configuration
	config.Load()

	// Configure logger from config
	logger.SetLevelFromString(config.Global.LogLevel)

	// Initialize Database
	logger.Info("Connecting to database...")
	database.Connect(config.Global.DatabasePath)

	if mode == "recheck" {
		if providerFilter == "" {
			providerFilter = "imagetwist"
		}
		fmt.Printf("Rechecking downloaded images for provider(s) containing: %s\n", providerFilter)
		if err := services.RecheckDownloadedImages(providerFilter); err != nil {
			fmt.Fprintf(os.Stderr, "Recheck failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Default: purge placeholders
	fmt.Println("Scanning for placeholder images...")
	result, err := services.ScanAndPurgePlaceholders()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Purge failed: %v\n", err)
		os.Exit(1)
	}

	// Pretty-print result
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode result: %v\n", err)
		os.Exit(1)
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
	}

	// Print known placeholder hashes hint
	fmt.Println("\n---")
	fmt.Println("To add known placeholder hashes, edit KnownPlaceholderHashes in services/image_service.go")
	fmt.Println("Format: \"provider:sha256hex\": \"provider\" or \":sha256hex\": \"provider\"")
	fmt.Println("Run this tool with --recheck --provider imagetwist to re-download any placeholders found.")
}
