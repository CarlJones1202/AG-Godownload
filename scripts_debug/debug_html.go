package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
)

func main() {
	content, err := ioutil.ReadFile("jkforum_debug.html")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	data := string(content)
	re := regexp.MustCompile(`<script type="application/json" data-nuxt-data="nuxt-app" data-ssr="true" id="__NUXT_DATA__">(.*?)</script>`)
	match := re.FindStringSubmatch(data)
	if len(match) > 1 {
		err = ioutil.WriteFile("nuxt_raw.json", []byte(match[1]), 0644)
		if err != nil {
			fmt.Println("Error writing file:", err)
		} else {
			fmt.Println("Wrote nuxt_raw.json")
		}
	} else {
		fmt.Println("Nuxt Data not found")
	}

	// Find URLs with image extensions
	reImgUrl := regexp.MustCompile(`https?:[^"'\s>]+\.(jpg|jpeg|png|gif|webp)`)
	matches := reImgUrl.FindAllString(data, -1)
	fmt.Println("--- IMAGE URLS ---")
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m] {
			fmt.Println(m)
			seen[m] = true
		}
	}

	// Find mymypic/mymyatt specifically
	reMy := regexp.MustCompile(`[^"'\s>]*mymy(pic|att)\.net[^"'\s>]*`)
	myMatches := reMy.FindAllString(data, -1)
	fmt.Println("\n--- MYMYPIC/MYMYATT URLS ---")
	for _, m := range myMatches {
		if !seen[m] {
			fmt.Println(m)
			seen[m] = true
		}
	}

	fmt.Println("--- IMG TAGS ---")
	reImg := regexp.MustCompile(`<img[^>]+>`)
	imgMatches := reImg.FindAllString(data, -1)
	for _, m := range imgMatches {
		fmt.Println(m)
	}

	fmt.Println("\n--- A TAGS WITH IMAGES ---")
	reA := regexp.MustCompile(`<a[^>]+>.*?<img[^>]+>.*?</a>`)
	aMatches := reA.FindAllString(data, -1)
	for _, m := range aMatches {
		fmt.Println(m)
	}

	fmt.Println("\n--- A TAGS ---")
	reAllA := regexp.MustCompile(`<a[^>]+>.*?</a>`)
	allAMatches := reAllA.FindAllString(data, -1)
	for _, m := range allAMatches {
		if strings.Contains(m, "mymypic") || strings.Contains(m, "mymyatt") {
			fmt.Println(m)
		}
	}
}
