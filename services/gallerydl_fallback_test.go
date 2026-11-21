package services

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/disintegration/imaging"
)

func TestResolveGalleryDLCommandFindsInstalledExecutable(t *testing.T) {
	command, args, err := resolveGalleryDLCommand("gallery-dl")
	if err != nil {
		t.Fatalf("gallery-dl should be resolved after installation: %v", err)
	}
	if command == "" {
		t.Fatal("resolved gallery-dl command is empty")
	}
	if len(args) != 0 {
		t.Fatalf("executable resolution should not use module args: %v", args)
	}
}

func TestDownloadImageWithIMXFallbackUsesDirectURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		img := imaging.New(500, 500, color.RGBA{R: 100, G: 120, B: 140, A: 255})
		if err := imaging.Encode(w, img, imaging.JPEG); err != nil {
			t.Errorf("failed to write synthetic image: %v", err)
		}
	}))
	defer server.Close()

	previousUploadsDir := UploadsDir
	UploadsDir = t.TempDir()
	defer func() { UploadsDir = previousUploadsDir }()

	result, err := DownloadImageWithIMXFallback(context.Background(), "", server.URL+"/image.jpg", "synthetic")
	if err != nil {
		t.Fatalf("direct IMX fallback failed: %v", err)
	}
	if result == nil || result.Path == "" {
		t.Fatal("direct IMX fallback returned no path")
	}
}

func TestDeriveIMXPageURL(t *testing.T) {
	tests := []struct {
		directURL string
		want      string
	}{
		{
			directURL: "https://image.imx.to/u/i/2024/11/24/5okw4a.jpg",
			want:      "https://imx.to/i/5okw4a",
		},
		{
			directURL: "https://i02.imx.to/i/2024/11/24/5okob5.jpg",
			want:      "https://imx.to/i/5okob5",
		},
	}
	for _, tt := range tests {
		if got := deriveIMXPageURL("", tt.directURL); got != tt.want {
			t.Errorf("deriveIMXPageURL(%q) = %q, want %q", tt.directURL, got, tt.want)
		}
	}
}

func TestIMXPageRefererPrefersResolvedPage(t *testing.T) {
	got := imxPageReferer("https://forum.example/thread/1", "https://image.imx.to/u/i/2024/11/24/5okob5.jpg")
	want := "https://imx.to/i/5okob5"
	if got != want {
		t.Fatalf("IMX referer = %q, want %q", got, want)
	}
}

func TestImageDownloadRefererUsesOriginalImageTwistPage(t *testing.T) {
	const pageURL = "https://imagetwist.com/abc123"
	if got := imageDownloadReferer("imagetwist", pageURL, "https://i1.imagetwist.com/image.jpg"); got != pageURL {
		t.Fatalf("ImageTwist referer = %q, want %q", got, pageURL)
	}
}
