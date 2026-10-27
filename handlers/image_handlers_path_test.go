package handlers

import (
	"path/filepath"
	"runtime"
	"testing"

	"gallery_api/services"
)

func TestResolveUploadPath(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{path: filepath.Join("source", "image.jpg"), ok: true},
		{path: filepath.Join("..", "outside.jpg"), ok: false},
		{path: filepath.Join("source", "..", "..", "outside.jpg"), ok: false},
		{path: `C:\outside.jpg`, ok: false},
		{path: `image.jpg::$DATA`, ok: false},
		{path: filepath.Join("source", "name."), ok: false},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			path string
			ok   bool
		}{path: `..\outside.jpg`, ok: false})
	}

	for _, test := range tests {
		_, ok := resolveUploadPath(test.path)
		if ok != test.ok {
			t.Errorf("resolveUploadPath(%q) ok = %v, want %v", test.path, ok, test.ok)
		}
	}

	got, ok := resolveUploadPath(filepath.Join("source", "image.jpg"))
	if !ok {
		t.Fatal("resolveUploadPath rejected an in-root image path")
	}
	if want := filepath.Join(services.UploadsDir, "source", "image.jpg"); got != want {
		t.Errorf("resolveUploadPath returned %q, want %q", got, want)
	}
}
