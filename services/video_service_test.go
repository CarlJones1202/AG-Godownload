package services

import "testing"

func TestIsGoogleDriveURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "https://drive.google.com/file/d/example/view", want: true},
		{url: "https://DRIVE.GOOGLE.COM/uc?id=example", want: true},
		{url: "https://example.com/video.mp4", want: false},
	}

	for _, tt := range tests {
		if got := isGoogleDriveURL(tt.url); got != tt.want {
			t.Errorf("isGoogleDriveURL(%q) = %t, want %t", tt.url, got, tt.want)
		}
	}
}
