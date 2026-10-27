package services

import (
	"strings"
	"testing"
)

// TestRipTurboImgLivePage exercises the real Colly code path against
// TurboImageHost, which returns 403 Forbidden to page visits that lack a
// Referer header (regardless of User-Agent). Requires network access;
// skipped when running with -short.
func TestRipTurboImgLivePage(t *testing.T) {
	if testing.Short() {
		t.Skip("network test: skipped with -short")
	}

	const src = "https://www.turboimagehost.com/p/85632310/015.jpg.html"

	imageURL, err := RipTurboImg(src)
	if err != nil {
		t.Fatalf("RipTurboImg returned error (was 403 Forbidden before the Referer fix?): %v", err)
	}
	if imageURL == "" {
		t.Fatal("RipTurboImg returned an empty image URL")
	}
	if !strings.Contains(imageURL, "turboimg.net") {
		t.Errorf("extracted URL does not look like a TurboImageHost CDN image: %s", imageURL)
	}
	t.Logf("extracted image URL: %s", imageURL)
}
