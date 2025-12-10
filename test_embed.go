package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	url := "https://www.pornhub.com/view_video.php?viewkey=65f2e0a9ac8c9"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	// Real Chrome UA
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	// Kitchen sink cookies
	cookies := []string{
		"accessAgeDisclaimerPH=1",
		"accessAgeDisclaimerUS=1",
		"accessAgeDisclaimer=1",
		"age_verified=1",
		"hasVerify=1",
		"hasVisited=1",
		"bs=1",                 // sometimes used
		"RNLBSERVERID=ded8396", // from the HTML I saw earlier
	}
	req.Header.Set("Cookie", strings.Join(cookies, "; "))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	bodyStr := string(body)
	fmt.Printf("Status Code: %d\n", resp.StatusCode)

	if strings.Contains(bodyStr, "Please verify your age") {
		fmt.Println("FAILURE: Still blocked by Age Verification")
	} else if strings.Contains(bodyStr, "flashvars") {
		fmt.Println("SUCCESS: Cookies worked!")
		if idx := strings.Index(bodyStr, "var flashvars_"); idx != -1 {
			fmt.Println(bodyStr[idx:idx+100] + "...")
		}
	} else {
		fmt.Println("UNKNOWN: No Age Gate, but no flashvars found.")
		// os.WriteFile("cookie_dump.html", body, 0644)
	}
}
