package services

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeXHamsterURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare id form used by internal links",
			in:   "https://xhamster.com/videos/xhwsahW",
			want: "https://xhamster.com/videos/video-xhwsahW",
		},
		{
			name: "slug form is left alone",
			in:   "https://xhamster.com/videos/high-heels-black-stockings-hot-brunette-gives-deep-throat-xhwsahW",
			want: "https://xhamster.com/videos/high-heels-black-stockings-hot-brunette-gives-deep-throat-xhwsahW",
		},
		{
			name: "numeric id form",
			in:   "https://xhamster.com/videos/8360949",
			want: "https://xhamster.com/videos/video-8360949",
		},
		{
			name: "movies url untouched",
			in:   "https://xhamster.com/movies/12345/some-slug.html",
			want: "https://xhamster.com/movies/12345/some-slug.html",
		},
		{
			name: "non xhamster url untouched",
			in:   "https://example.com/videos/abc",
			want: "https://example.com/videos/abc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeXHamsterURL(tc.in); got != tc.want {
				t.Errorf("normalizeXHamsterURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestXhamsterInitialsParsing(t *testing.T) {
	page := `<html><script>window.initials = {"videoModel":{"title":"T","sources":null},"xplayerSettings":null,"avPlaceholderComponent":{"title":"To watch this video, you must<br>verify your age and be logged in"}};</script></html>`

	match := xhamsterInitialsRe.FindStringSubmatch(page)
	if match == nil {
		t.Fatal("window.initials regex did not match")
	}

	info := &xhamsterPageInfo{}
	if err := json.Unmarshal([]byte(match[1]), info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if info.hasFormats() {
		t.Error("expected no formats for a sources:null page")
	}
	reason := info.noFormatsReason()
	if reason == "" {
		t.Fatal("expected a no-formats reason for a sources:null page")
	}
	if !strings.Contains(reason, "logged-in session") {
		t.Errorf("reason should mention the login requirement, got %q", reason)
	}
	if strings.Contains(reason, "<br>") {
		t.Errorf("reason should not contain raw HTML, got %q", reason)
	}
}

func TestXhamsterPageWithSourcesHasNoReason(t *testing.T) {
	info := &xhamsterPageInfo{}
	if err := json.Unmarshal([]byte(`{"videoModel":{"title":"T","sources":{"h264":["https://example.com/v.mp4"]}},"xplayerSettings":null}`), info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !info.hasFormats() {
		t.Error("expected formats for a page with sources")
	}
	if reason := info.noFormatsReason(); reason != "" {
		t.Errorf("expected no reason for a downloadable page, got %q", reason)
	}

	xplayer := &xhamsterPageInfo{}
	if err := json.Unmarshal([]byte(`{"videoModel":{"title":"T","sources":null},"xplayerSettings":{"sources":{"standard":{"h264":[]}}}}`), xplayer); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !xplayer.hasFormats() {
		t.Error("expected formats for a page with xplayerSettings.sources")
	}
}
