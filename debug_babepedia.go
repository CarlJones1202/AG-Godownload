package main

import (
	"fmt"
	"gallery_api/services"
	"log"
)

func main() {
	s := services.NewBabepediaService()
	// Test with a known performer
	name := "Mia Malkova"
	fmt.Printf("Searching for %s...\n", name)

	results, err := s.Search(name)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		log.Fatal("No results found")
	}

	fmt.Printf("Found %d results. Getting details for first result: %s (ID: %s)\n", len(results), results[0].Name, results[0].ExternalID)

	details, err := s.GetDetails(results[0].ExternalID)
	if err != nil {
		log.Fatalf("GetDetails failed: %v", err)
	}

	fmt.Printf("Name: %s\n", details.Name)
	fmt.Printf("Birthdate: '%s'\n", details.Birthdate)
	fmt.Printf("Country: '%s'\n", details.Country)
	fmt.Printf("Measurements: '%s'\n", details.Measurements)
	fmt.Printf("Photos found: %d\n", len(details.Photos))
	if len(details.Photos) > 0 {
		fmt.Printf("First Photo: %s\n", details.Photos[0])
	}
}
