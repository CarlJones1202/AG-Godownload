package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestParseVipergirlsSearchExtractsResultCards(t *testing.T) {
	base, err := url.Parse(vipergirlsBaseURL + "/")
	if err != nil {
		t.Fatal(err)
	}

	page := `
		<ol id="searchbits" start="1" class="searchbits">
			<li class="imodselector threadbit" id="thread_12345">
				<div class="threadinfo thread">
					<h3 class="searchtitle">
						<a class="title thread-preview" id="thread_title_12345"
						   href="threads/12345-Example-Gallery?s=abc123&amp;highlight=example"
						   data-images="[&quot;https:\/\/img.example.com\/first.jpg&quot;,&quot;https:\/\/img.example.com\/second.jpg&quot;]">Example Gallery</a>
					</h3>
				</div>
				<div class="threadpostedin td alt">
					<p>Forum:<br /><a href="forums/235-Photo-Galleries" title="Photo Galleries">Photo Galleries</a></p>
				</div>
			</li>
			<li class="imodselector threadbit" id="thread_67890">
				<h3 class="searchtitle"><a class="title thread-preview" href="/threads/67890-Second-Set">Second Set</a></h3>
			</li>
			<li class="imodselector threadbit" id="thread_11111">
				<h3 class="searchtitle"><a class="title thread-preview" href="https://vipergirls.to/threads/11111-Third-Set">Third Set</a></h3>
				<div class="threadpostedin"><a>Photo Link Drop</a></div>
			</li>
			<li class="imodselector threadbit" id="thread_22222">
				<h3 class="searchtitle"><a class="title thread-preview" href="https://vipergirls.to/threads/22222-Video-Set">Video Set</a></h3>
				<div class="threadpostedin"><a>Hardcore Videos</a></div>
			</li>
			<li class="imodselector threadbit" id="thread_33333">
				<h3 class="searchtitle"><a class="title thread-preview" href="https://vipergirls.to/threads/33333-Magazine-Set">Magazine Set</a></h3>
				<div class="threadpostedin"><a>Adult Magazines</a></div>
			</li>
		</ol>`

	results, err := ParseVipergirlsSearch(strings.NewReader(page), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3 photo-forum results", len(results))
	}

	first := results[0]
	if first.ThreadID != "12345" || first.Title != "Example Gallery" {
		t.Errorf("unexpected first result identity: %+v", first)
	}
	if first.SubForum != "Photo Galleries" {
		t.Errorf("SubForum = %q, want %q", first.SubForum, "Photo Galleries")
	}
	if first.ThreadURL != "https://vipergirls.to/threads/12345-Example-Gallery" {
		t.Errorf("ThreadURL = %q, want canonical URL without session/highlight", first.ThreadURL)
	}
	if first.ThumbnailURL != "https://img.example.com/first.jpg" {
		t.Errorf("ThumbnailURL = %q, want first data-images entry", first.ThumbnailURL)
	}

	if results[1].ThreadID != "11111" || results[1].SubForum != "Photo Link Drop" {
		t.Errorf("unexpected second result: %+v", results[1])
	}
	if results[1].ThumbnailURL != "" {
		t.Errorf("result without data-images should have no thumbnail, got %q", results[1].ThumbnailURL)
	}
	if results[2].ThreadID != "33333" || results[2].SubForum != "Adult Magazines" {
		t.Errorf("unexpected third result: %+v", results[2])
	}
	for _, result := range results {
		if result.ThreadID == "67890" || result.ThreadID == "22222" {
			t.Errorf("result %q should have been filtered out", result.ThreadID)
		}
	}
}

func TestParseVipergirlsSearchSkipsPrefixLinksBeforeThreadTitle(t *testing.T) {
	base, _ := url.Parse(vipergirlsBaseURL + "/")
	page := `<ol id="searchbits">
		<li class="imodselector threadbit" id="thread_16406235">
			<h3 class="searchtitle">
				<span id="thread_prefix_16406235" class="prefix understate">
					<b>[</b><a class="title" href="forumdisplay.php?s=abc&amp;f=302&amp;prefixid=MetArtX" title="Show all MetArtX threads.">MetArtX</a><b>]</b>
				</span>
				<a class="title thread-preview" href="threads/16406235-Leya-Desantis-Pleasure-s-Doors-1-x122-(09-28-26)?s=abc&amp;highlight=leya+desantis"
				   id="thread_title_16406235"
				   data-images="[&quot;https:\/\/image.imx.to\/u\/t\/2026\/09\/28\/768lu4.jpg&quot;]">Leya Desantis - Pleasure's Doors 1 - x122 - (09/28/26)</a>
			</h3>
			<div class="threadpostedin td alt">
				<p>Forum:<br /><a href="forums/302-Softcore-Photo-Sets?s=abc" title="Softcore Photo Sets">Softcore Photo Sets</a></p>
			</div>
		</li>
	</ol>`

	results, err := ParseVipergirlsSearch(strings.NewReader(page), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	got := results[0]
	if got.ThreadID != "16406235" {
		t.Errorf("ThreadID = %q, want %q", got.ThreadID, "16406235")
	}
	if got.Title != "Leya Desantis - Pleasure's Doors 1 - x122 - (09/28/26)" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.SubForum != "Softcore Photo Sets" {
		t.Errorf("SubForum = %q, want %q", got.SubForum, "Softcore Photo Sets")
	}
	if got.ThreadURL != "https://vipergirls.to/threads/16406235-Leya-Desantis-Pleasure-s-Doors-1-x122-(09-28-26)" {
		t.Errorf("ThreadURL = %q", got.ThreadURL)
	}
	if got.ThumbnailURL != "https://image.imx.to/u/t/2026/09/28/768lu4.jpg" {
		t.Errorf("ThumbnailURL = %q", got.ThumbnailURL)
	}
}

func TestIsVipergirlsPhotoForum(t *testing.T) {
	tests := []struct {
		subForum string
		want     bool
	}{
		{"Adult Photo Link Drop", true},
		{"Photo Galleries", true},
		{"Adult Magazines", true},
		{"Adult Comics", true},
		{"Hardcore Videos", false},
		{"Fetish Videos", false},
		{"Adult Movies", false},
		{"Transsexual Video Collections", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isVipergirlsPhotoForum(tt.subForum); got != tt.want {
			t.Errorf("isVipergirlsPhotoForum(%q) = %v, want %v", tt.subForum, got, tt.want)
		}
	}
}

func TestFirstVipergirlsImagePrefersThumbnailURL(t *testing.T) {
	raw := `["https://img3.pixhost.cc/images/7042/full.jpg","https://t3.pixhost.cc/thumbs/7042/preview.jpg"]`
	got := firstVipergirlsImage(raw)
	if got != "https://t3.pixhost.cc/thumbs/7042/preview.jpg" {
		t.Fatalf("firstVipergirlsImage() = %q, want thumbnail URL", got)
	}
	if got := firstVipergirlsImage(`["https://img.example.com/only.jpg"]`); got != "https://img.example.com/only.jpg" {
		t.Fatalf("fallback image = %q, want first image", got)
	}
}

func TestParseVipergirlsSearchIgnoresNonThreadLinks(t *testing.T) {
	base, _ := url.Parse(vipergirlsBaseURL + "/")
	page := `<ol id="searchbits">
		<li class="threadbit"><h3 class="searchtitle"><a class="title" href="https://evil.example/threads/1-Bad">Bad</a></h3></li>
		<li class="threadbit"><h3 class="searchtitle"><a class="title" href="forums/235-Photo-Galleries">Forum</a></h3></li>
	</ol>`

	results, err := ParseVipergirlsSearch(strings.NewReader(page), base)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}

func TestIsVipergirlsThreadURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"canonical thread", "https://vipergirls.to/threads/12345-Example-Gallery", true},
		{"www host", "https://www.vipergirls.to/threads/12345", true},
		{"http rejected", "http://vipergirls.to/threads/12345", false},
		{"lookalike host rejected", "https://vipergirls.to.evil.example/threads/12345", false},
		{"forum page rejected", "https://vipergirls.to/forums/235-Photo-Galleries", false},
		{"relative rejected", "/threads/12345", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsVipergirlsThreadURL(tt.url); got != tt.want {
				t.Fatalf("IsVipergirlsThreadURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestScrapeVipergirlsDoesNotWaitForThumbnailDownload(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:vg-scrape-thumb-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	originalDB := database.DB
	originalUploads := UploadsDir
	database.DB = testDB
	UploadsDir = t.TempDir()
	t.Cleanup(func() {
		database.DB = originalDB
		UploadsDir = originalUploads
		_ = sqlDB.Close()
	})
	if err := testDB.AutoMigrate(
		&models.Person{}, &models.Gallery{}, &models.Source{}, &models.Image{},
		&models.PersonProviderAlias{}, &models.ScanResultExclusion{}, &models.PersonScanQueue{},
	); err != nil {
		t.Fatal(err)
	}

	person := models.Person{Name: "Test Person"}
	if err := testDB.Create(&person).Error; err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseDownload := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseDownload()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		case <-time.After(3 * time.Second):
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake png bytes"))
	}))
	defer server.Close()

	threadURL := "https://vipergirls.to/threads/16406235-Leya-Desantis-Pleasure-s-Doors-1"
	thumbnailURL := server.URL + "/thumb.png"
	start := time.Now()
	response, err := ScrapeVipergirlsAsMissingGallery(ScrapeVipergirlsRequest{
		PersonID:     person.ID,
		Provider:     "metart",
		GalleryName:  "Pleasure's Doors 1",
		ThreadURL:    threadURL,
		ThumbnailURL: thumbnailURL,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || response.GalleryID == 0 {
		t.Fatalf("unexpected scrape response: %+v", response)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("scrape took %v; thumbnail download must not block the response", elapsed)
	}

	var gallery models.Gallery
	if err := testDB.First(&gallery, response.GalleryID).Error; err != nil {
		t.Fatal(err)
	}
	if gallery.ProviderThumbnailURL != thumbnailURL {
		t.Errorf("ProviderThumbnailURL = %q, want %q", gallery.ProviderThumbnailURL, thumbnailURL)
	}
	if gallery.ProviderThumbnail != "" {
		t.Errorf("ProviderThumbnail = %q, want empty until background download finishes", gallery.ProviderThumbnail)
	}

	releaseDownload()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := testDB.First(&gallery, response.GalleryID).Error; err != nil {
			t.Fatal(err)
		}
		if gallery.ProviderThumbnail != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if gallery.ProviderThumbnail == "" {
		t.Error("background thumbnail download did not cache a local path")
	}
}

func TestVipergirlsSearchURLUsesPhotoSectionSearchParameters(t *testing.T) {
	searchURL := VipergirlsSearchURL(`"Jane Doe" "Some Gallery"`)
	parsed, err := url.Parse(searchURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "vipergirls.to" || parsed.Path != "/search.php" {
		t.Fatalf("unexpected search URL: %s", searchURL)
	}
	query := parsed.Query()
	if query.Get("do") != "process" ||
		query.Get("contenttype") != "1" ||
		query.Get("titleonly") != "1" ||
		query.Get("imgsearch") != "1" ||
		query.Get("childforums") != "1" {
		t.Errorf("unexpected search parameters: %v", query)
	}
	if got := query["forumchoice[]"]; len(got) != len(vipergirlsPhotoForumIDs) {
		t.Errorf("forumchoice[] = %v, want %v", got, vipergirlsPhotoForumIDs)
	} else {
		for i, want := range vipergirlsPhotoForumIDs {
			if got[i] != want {
				t.Errorf("forumchoice[%d] = %q, want %q", i, got[i], want)
			}
		}
	}
	if query.Get("query") != `"Jane Doe" "Some Gallery"` {
		t.Errorf("query = %q", query.Get("query"))
	}
}
