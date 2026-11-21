package services

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
)

func TestCheckHotlinkPlaceholderRejectsSmallUndecodableIMXResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "response.html")
	if err := os.WriteFile(path, []byte("<html>blocked</html>"), 0644); err != nil {
		t.Fatal(err)
	}

	isPlaceholder, reason, err := IsHotlinkPlaceholder(path, "imx")
	if err != nil {
		t.Fatalf("placeholder check failed: %v", err)
	}
	if !isPlaceholder {
		t.Fatal("small undecodable IMX response should be rejected")
	}
	if reason == "" {
		t.Fatal("placeholder rejection should include a reason")
	}
}

func TestCheckHotlinkPlaceholderRejectsSmallImageBamResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placeholder.png")
	img := imaging.New(240, 240, color.RGBA{R: 220, G: 220, B: 220, A: 255})
	if err := imaging.Save(img, path); err != nil {
		t.Fatal(err)
	}

	isPlaceholder, reason, err := IsHotlinkPlaceholder(path, "imagebam")
	if err != nil {
		t.Fatalf("placeholder check failed: %v", err)
	}
	if !isPlaceholder {
		t.Fatal("small ImageBam response should be rejected")
	}
	if reason == "" {
		t.Fatal("placeholder rejection should include a reason")
	}
}
