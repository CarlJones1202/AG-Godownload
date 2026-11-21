package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseKittyKatsSearchExtractsPhotoThreads(t *testing.T) {
	base, err := url.Parse(kittyKatsBaseURL + "/")
	if err != nil {
		t.Fatal(err)
	}

	page := `
		<li class="block-row block-row--separated">
			<div class="contentRow-main">
				<h3 class="contentRow-title">
					<a href="/threads/mila-azul-ahre-4324px-127-pictures-07-may-2018.2806773/">
						<span class="label label--primary">SexArt</span>
						<em>Mila</em> <em>Azul</em> - <em>Ahre</em> - 4324px
					</a>
				</h3>
				<div class="contentRow-minor">
					<ul class="listInline">
						<li><a href="/members/qwz75.10114370/">qwz75</a></li>
						<li>Forum: <a href="/forums/picture-thumbs-posts-rar-files.15/">Picture (Thumbs) Posts &amp; RAR-files</a></li>
					</ul>
				</div>
			</div>
		</li>
		<li class="block-row">
			<div class="contentRow-main">
				<h3 class="contentRow-title">
					<a href="/threads/example-gallery.123/?highlight=example">Example Gallery</a>
				</h3>
				<div class="contentRow-minor"><a href="/forums/photo-galleries.16/">Photo Galleries</a></div>
			</div>
		</li>
		<li class="block-row">
			<div class="contentRow-main">
				<h3 class="contentRow-title"><a href="/threads/video-set.999/">Video Set</a></h3>
				<div class="contentRow-minor"><a href="/forums/videos.17/">Adult Videos</a></div>
			</div>
		</li>`

	results, err := ParseKittyKatsSearch(strings.NewReader(page), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 photo-forum results", len(results))
	}

	first := results[0]
	if first.Site != ForumSiteKittyKats || first.ThreadID != "2806773" {
		t.Errorf("unexpected first result identity: %+v", first)
	}
	if first.ThreadURL != "https://kitty-kats.net/threads/mila-azul-ahre-4324px-127-pictures-07-may-2018.2806773" {
		t.Errorf("ThreadURL = %q, want canonical URL without trailing slash", first.ThreadURL)
	}
	if first.SubForum != "Picture (Thumbs) Posts & RAR-files" {
		t.Errorf("SubForum = %q", first.SubForum)
	}
	if !strings.Contains(first.Title, "SexArt") || !strings.Contains(first.Title, "Ahre") {
		t.Errorf("Title = %q, want label and title text", first.Title)
	}
	if results[1].ThreadURL != "https://kitty-kats.net/threads/example-gallery.123" {
		t.Errorf("second ThreadURL = %q, want query/highlight removed", results[1].ThreadURL)
	}
	for _, result := range results {
		if strings.Contains(strings.ToLower(result.SubForum), "video") {
			t.Errorf("video result was not filtered: %+v", result)
		}
	}
}

func TestIsKittyKatsThreadURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"canonical thread", "https://kitty-kats.net/threads/example-gallery.123/", true},
		{"www host", "https://www.kitty-kats.net/threads/example-gallery.123", true},
		{"http rejected", "http://kitty-kats.net/threads/example-gallery.123", false},
		{"lookalike host rejected", "https://kitty-kats.net.evil.example/threads/example-gallery.123", false},
		{"forum page rejected", "https://kitty-kats.net/forums/picture-galleries.15", false},
		{"relative rejected", "/threads/example-gallery.123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKittyKatsThreadURL(tt.url); got != tt.want {
				t.Fatalf("IsKittyKatsThreadURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
			if got := IsForumThreadURL(tt.url); got != tt.want {
				t.Fatalf("IsForumThreadURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
	if !IsForumThreadURL("https://vipergirls.to/threads/12345-Example") {
		t.Error("IsForumThreadURL should accept vipergirls.to threads")
	}
}

func TestFillKittyKatsThumbnailsUsesFirstPostImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<article class="message--post">
				<div class="message-userContent"><div class="bbWrapper">
					<a href="/posts/1"><img src="/avatar.jpg" alt="View Post"></a>
					<a href="/image.jpg"><img src="/thumb.jpg" data-url="/thumb.jpg"></a>
				</div></div>
			</article>`))
	}))
	defer server.Close()

	results := []ForumSearchResult{{
		Site:      ForumSiteKittyKats,
		ThreadID:  "123",
		ThreadURL: server.URL + "/threads/example-gallery.123/",
	}}
	fillKittyKatsThumbnails(context.Background(), server.Client(), results)

	want := server.URL + "/thumb.jpg"
	if results[0].ThumbnailURL != want {
		t.Fatalf("ThumbnailURL = %q, want %q", results[0].ThumbnailURL, want)
	}
}

func TestCollectForumSearchKeepsResultsWhenOneSiteFails(t *testing.T) {
	vgResult := ForumSearchResult{
		Site:      ForumSiteVipergirls,
		ThreadID:  "12345",
		ThreadURL: "https://vipergirls.to/threads/12345-Example",
	}
	response, err := collectForumSearch(
		context.Background(),
		`"Example Gallery"`,
		func(_ context.Context, _ string) (*VipergirlsSearchResponse, error) {
			return &VipergirlsSearchResponse{Data: []ForumSearchResult{vgResult}}, nil
		},
		func(_ context.Context, _ string) ([]ForumSearchResult, error) {
			return nil, fmt.Errorf("kitty-kats search returned status 403")
		},
	)
	if err != nil {
		t.Fatalf("one failed site should not fail the whole search: %v", err)
	}
	if len(response.Data) != 1 || response.Data[0].ThreadURL != vgResult.ThreadURL {
		t.Fatalf("Data = %+v, want the vipergirls result", response.Data)
	}
	if len(response.Sources) != 2 {
		t.Fatalf("Sources = %+v, want both sites", response.Sources)
	}
	if response.Sources[0].Error != "" {
		t.Errorf("vipergirls source error = %q, want empty", response.Sources[0].Error)
	}
	if !strings.Contains(response.Sources[1].Error, "403") {
		t.Errorf("kitty-kats source error = %q, want status 403", response.Sources[1].Error)
	}

	response, err = collectForumSearch(
		context.Background(),
		`"Example Gallery"`,
		func(_ context.Context, _ string) (*VipergirlsSearchResponse, error) {
			return nil, fmt.Errorf("vipergirls unavailable")
		},
		func(_ context.Context, _ string) ([]ForumSearchResult, error) {
			return nil, fmt.Errorf("kitty-kats unavailable")
		},
	)
	if err == nil {
		t.Fatal("both sites failing should return an error")
	}
	if response == nil || len(response.Sources) != 2 {
		t.Fatalf("response = %+v, want source errors for both sites", response)
	}
}

func TestKittyKatsSearchMarkupStaysCompatible(t *testing.T) {
	// Guards the selectors used by the live search against small markup changes.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<li class="block-row"><h3 class="contentRow-title"><a href="/threads/example.456/">Example</a></h3>
		<div class="contentRow-minor"><a href="/forums/photo.15/">Photo Galleries</a></div></li>`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Find(`li.block-row h3.contentRow-title > a`).Length() != 1 {
		t.Error("title selector no longer matches a kitty-kats result")
	}
	if doc.Find(`.contentRow-minor a[href*="/forums/"]`).First().Text() != "Photo Galleries" {
		t.Error("sub forum selector no longer matches a kitty-kats result")
	}
}
