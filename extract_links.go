package main_debug

import (
	"fmt"
	"io/ioutil"
	"regexp"
)

func main() {
	content, err := ioutil.ReadFile("jkforum_sample.html")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	data := string(content)
	re := regexp.MustCompile(`class="[^"]*post[^"]*"`)
	matches := re.FindAllString(data, -1)
	for _, m := range matches {
		fmt.Println("Found post-related class:", m)
	}
}
