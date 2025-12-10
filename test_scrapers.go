package main

import (
	"fmt"
	"log"

	"gallery_api/services"
)

func main() {
	// Test Babepedia
	fmt.Println("=== Testing Babepedia ===")
	babepedia := services.NewBabepediaService()

	fmt.Println("Searching for 'Mia Malkova'...")
	babepediaResults, err := babepedia.Search("Mia Malkova")
	if err != nil {
		log.Printf("Babepedia search error: %v\n", err)
	} else {
		fmt.Printf("Found %d results\n", len(babepediaResults))
		for i, result := range babepediaResults {
			fmt.Printf("  %d. %s (ID: %s) - %s\n", i+1, result.Name, result.ExternalID, result.Disambiguation)
		}

		if len(babepediaResults) > 0 {
			fmt.Printf("\nFetching details for: %s\n", babepediaResults[0].ExternalID)
			details, err := babepedia.GetDetails(babepediaResults[0].ExternalID)
			if err != nil {
				log.Printf("Babepedia details error: %v\n", err)
			} else {
				fmt.Printf("Name: %s\n", details.Name)
				fmt.Printf("Birthdate: %s\n", details.Birthdate)
				fmt.Printf("Country: %s\n", details.Country)
				fmt.Printf("Photos: %d\n", len(details.Photos))
			}
		}
	}

	fmt.Println("\n=== Testing FreeOnes ===")
	freeones := services.NewFreeOnesService()

	fmt.Println("Searching for 'Mia Malkova'...")
	freeonesResults, err := freeones.Search("Mia Malkova")
	if err != nil {
		log.Printf("FreeOnes search error: %v\n", err)
	} else {
		fmt.Printf("Found %d results\n", len(freeonesResults))
		for i, result := range freeonesResults {
			fmt.Printf("  %d. %s (ID: %s)\n", i+1, result.Name, result.ExternalID)
		}

		if len(freeonesResults) > 0 {
			fmt.Printf("\nFetching details for: %s\n", freeonesResults[0].ExternalID)
			details, err := freeones.GetDetails(freeonesResults[0].ExternalID)
			if err != nil {
				log.Printf("FreeOnes details error: %v\n", err)
			} else {
				fmt.Printf("Name: %s\n", details.Name)
				fmt.Printf("Birthdate: %s\n", details.Birthdate)
				fmt.Printf("Country: %s\n", details.Country)
				fmt.Printf("Photos: %d\n", len(details.Photos))
			}
		}
	}
}
