package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
)

func main() {
	content, err := ioutil.ReadFile("jkforum_debug.html")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	data := string(content)

	// Find anything that looks like a mymypic/mymyatt domain or path
	re := regexp.MustCompile(`[^\s"'<>]*mymy(pic|att)\.[^\s"'<>]*`)
	matches := re.FindAllString(data, -1)

	fmt.Println("--- FOUND LINKS ---")
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m] {
			fmt.Println(m)
			seen[m] = true
		}
	}
}
