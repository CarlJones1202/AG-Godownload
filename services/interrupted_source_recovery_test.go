package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCountPostImageLinksUsesCrawlerPostSelection(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div id="post_message_12">
			<a href="photo-1"><img src="thumb-1"></a>
			<a href="view-post"><img alt="View Post"></a>
		</div>
		<div id="post_message_13"><a href="photo-2"><img src="thumb-2"></a></div>
	`))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		location string
		fragment string
		want     int
	}{
		{name: "default source selects first post", location: "https://example.com/thread", want: 1},
		{name: "fragment selects specific post", location: "https://example.com/thread", fragment: "post13", want: 1},
		{name: "unknown fragment returns no post", location: "https://example.com/thread", fragment: "post99", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countPostImageLinks(selectSourcePost(doc, tt.location, tt.fragment))
			if got != tt.want {
				t.Fatalf("countPostImageLinks() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSelectSourcePostUsesSiteSpecificMarkup(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<article class="message--post">
			<div class="message-userContent"><div class="bbWrapper">
				<a href="photo"><img src="thumb"></a>
			</div></div>
		</article>
		<div id="post_message_2"><a href="other"><img src="other-thumb"></a></div>
	`))
	if err != nil {
		t.Fatal(err)
	}

	got := countPostImageLinks(selectSourcePost(doc, "https://kitty-kats.net/thread", ""))
	if got != 1 {
		t.Fatalf("kitty-kats source image count = %d, want 1", got)
	}
}

func TestAuditGallerySourceQueuesOnlyWhenCountsDiffer(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:recover-source-audit-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	originalDB := database.DB
	database.DB = testDB
	t.Cleanup(func() {
		database.DB = originalDB
		_ = sqlDB.Close()
	})
	if err := testDB.AutoMigrate(&models.Source{}, &models.Gallery{}, &models.Image{}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<div id="post_message_1"><a href="photo"><img src="thumb"></a></div>`))
	}))
	defer server.Close()

	source := models.Source{Name: "source", Location: server.URL, Status: "idle"}
	if err := testDB.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	gallery := models.Gallery{Name: "gallery", SourceID: &source.ID}
	if err := testDB.Create(&gallery).Error; err != nil {
		t.Fatal(err)
	}
	firstImage := models.Image{GalleryID: gallery.ID, Type: "image", Filename: "first.jpg"}
	if err := testDB.Create(&firstImage).Error; err != nil {
		t.Fatal(err)
	}

	report, err := AuditGallerySource(gallery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.GalleryPhotos != 1 || report.SourcePhotos != 1 || report.SourceRequeued {
		t.Fatalf("matching count audit = %+v, want one photo and no requeue", report)
	}

	secondImage := models.Image{
		Type:     "image",
		Filename: "second.jpg",
		Galleries: []*models.Gallery{
			&gallery,
		},
	}
	if err := testDB.Create(&secondImage).Error; err != nil {
		t.Fatal(err)
	}

	report, err = AuditGallerySource(gallery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.GalleryPhotos != 2 || report.SourcePhotos != 1 || !report.SourceRequeued {
		t.Fatalf("mismatched count audit = %+v, want two saved, one in source, and requeue", report)
	}
	var queuedSource models.Source
	if err := testDB.First(&queuedSource, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if queuedSource.Status != "queued" {
		t.Fatalf("source status = %q, want queued", queuedSource.Status)
	}

	report, err = AuditGallerySource(gallery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.SourcePending || report.SourceRequeued {
		t.Fatalf("repeat mismatch audit = %+v, want existing queued source left pending", report)
	}
}

func TestAuditAllGallerySourcesSkipsIneligibleAndContinuesAfterFailure(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:recover-all-sources-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	originalDB := database.DB
	database.DB = testDB
	t.Cleanup(func() {
		database.DB = originalDB
		_ = sqlDB.Close()
	})
	if err := testDB.AutoMigrate(&models.Source{}, &models.Gallery{}, &models.Image{}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			_, _ = w.Write([]byte(`<div id="post_message_1"><a href="photo"><img src="thumb"></a></div>`))
		}
	}))
	defer server.Close()

	validSource := models.Source{Name: "valid", Location: server.URL + "/valid"}
	brokenSource := models.Source{Name: "broken", Location: server.URL + "/no-post"}
	localSource := models.Source{Name: "local", Location: `C:\photos\image.jpg`}
	for _, source := range []*models.Source{&validSource, &brokenSource, &localSource} {
		if err := testDB.Create(source).Error; err != nil {
			t.Fatal(err)
		}
	}

	galleries := []models.Gallery{
		{Name: "valid gallery", SourceID: &validSource.ID},
		{Name: "broken gallery", SourceID: &brokenSource.ID},
		{Name: "local gallery", SourceID: &localSource.ID},
		{Name: "manual gallery"},
	}
	for i := range galleries {
		if err := testDB.Create(&galleries[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	batch, err := AuditAllGallerySources()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Audits) != 1 || batch.Skipped != 2 || len(batch.Failures) != 1 {
		t.Fatalf("batch result = %+v, want one audit, two skipped, and one failure", batch)
	}
	if batch.Audits[0].GalleryID != galleries[0].ID || !batch.Audits[0].SourceRequeued {
		t.Fatalf("valid gallery audit = %+v, want gallery queued after count mismatch", batch.Audits[0])
	}
	if batch.Failures[0].GalleryID != galleries[1].ID {
		t.Fatalf("failed gallery ID = %d, want %d", batch.Failures[0].GalleryID, galleries[1].ID)
	}
}
