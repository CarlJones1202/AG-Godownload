package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRipImxSubmitsContinueButton(t *testing.T) {
	var postCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Method == http.MethodPost {
			postCount++
			if err := r.ParseForm(); err != nil {
				t.Errorf("failed to parse continue POST: %v", err)
			}
			if r.Form.Get("imgContinue") == "" {
				t.Error("continue POST omitted imgContinue")
			}
			fmt.Fprint(w, `<img src="https://image.imx.to/u/i/2024/11/24/test-image.jpg" />`)
			return
		}
		fmt.Fprint(w, `<button name="imgContinue" value="Continue to your image...">Continue</button>`)
	}))
	defer server.Close()

	imageURL, err := RipImx(server.URL)
	if err != nil {
		t.Fatalf("RipImx failed: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected one continue POST, got %d", postCount)
	}
	if imageURL != "https://image.imx.to/u/i/2024/11/24/test-image.jpg" {
		t.Fatalf("unexpected image URL: %q", imageURL)
	}
}
