package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRipImagetwistPrefersImageOverPremiumPoster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<img class="pic img-responsive" src="%s">`, serverURL(r)+"/i/15619/test-image.jpg/premium_poster.jpg")
		fmt.Fprintf(w, `<textarea>[img]%s[/img]</textarea>`, serverURL(r)+"/i/15619/test-image.jpg")
	}))
	defer server.Close()

	imageURL, err := RipImagetwist(server.URL + "/test-image/premium_poster.jpg")
	if err != nil {
		t.Fatalf("RipImagetwist failed: %v", err)
	}
	want := server.URL + "/i/15619/test-image.jpg"
	if imageURL != want {
		t.Fatalf("expected full image URL %q, got %q", want, imageURL)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
