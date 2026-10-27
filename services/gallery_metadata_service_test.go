package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestParsePlayboyPlusAlgoliaConfig(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	apiKey := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("validUntil=%d&restrictIndices=all%%2A", now.Add(time.Hour).Unix())))
	page := fmt.Sprintf(`<script>window.env = {"api":{"algolia":{"applicationID":"app-id","apiKey":"%s"}}};</script>`, apiKey)

	config, cacheUntil, err := parsePlayboyPlusAlgoliaConfig(page, now)
	if err != nil {
		t.Fatalf("parsePlayboyPlusAlgoliaConfig returned an error: %v", err)
	}
	if config.applicationID != "app-id" || config.apiKey != apiKey {
		t.Fatalf("unexpected Algolia config: %#v", config)
	}
	if want := now.Add(30 * time.Minute); !cacheUntil.Equal(want) {
		t.Fatalf("cache expiry = %s, want %s", cacheUntil, want)
	}
}

func TestParsePlayboyPlusAlgoliaConfigRejectsInvalidPage(t *testing.T) {
	_, _, err := parsePlayboyPlusAlgoliaConfig("<html></html>", time.Now())
	if err == nil {
		t.Fatal("expected an error for a page without Algolia configuration")
	}
}

func TestParsePlayboyPlusAlgoliaConfigRejectsExpiredKey(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	apiKey := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("validUntil=%d", now.Add(-time.Hour).Unix())))
	page := fmt.Sprintf(`window.env = {"api":{"algolia":{"applicationID":"app-id","apiKey":"%s"}}};`, apiKey)

	_, _, err := parsePlayboyPlusAlgoliaConfig(page, now)
	if err == nil {
		t.Fatal("expected an error for an expired Algolia key")
	}
}

func TestIsGalleryResultExcludesMovieURLs(t *testing.T) {
	if isGalleryResult(GallerySearchResult{Title: "The Movie Night", URL: "https://example.com/model/name/movie/20261001/THE_MOVIE_NIGHT"}) {
		t.Fatal("movie URL must not be treated as a missing gallery")
	}
	if !isGalleryResult(GallerySearchResult{Title: "Summer Set", URL: "https://example.com/model/name/gallery/20261001/SUMMER_SET"}) {
		t.Fatal("gallery URL should remain eligible")
	}
}

func TestPlayboyPlusGalleryNameRequiresPersonInTitle(t *testing.T) {
	galleryName, ok := PlayboyPlusGalleryName("Kate Jones in Exquisite Allure", "Kate Jones")
	if !ok || galleryName != "Exquisite Allure" {
		t.Fatalf("PlayboyPlusGalleryName = (%q, %t), want (%q, true)", galleryName, ok, "Exquisite Allure")
	}

	for _, title := range []string{
		"Kate Jones and Jane Doe in Exquisite Allure",
		"Exquisite Allure",
		"Kate Jones in ",
	} {
		if _, ok := PlayboyPlusGalleryName(title, "Kate Jones"); ok {
			t.Errorf("PlayboyPlusGalleryName(%q) unexpectedly accepted a mashup or invalid title", title)
		}
	}
}

func TestScanSourceForPersonRejectsVixenGalleryScan(t *testing.T) {
	_, err := ScanSourceForPerson(0, "Vixen", "")
	if err == nil {
		t.Fatal("expected Vixen gallery scan to return an explanatory error")
	}
}

func TestPlayboyPlusActorFacetParams(t *testing.T) {
	params, err := playboyPlusActorFacetParams(122245)
	if err != nil {
		t.Fatalf("playboyPlusActorFacetParams returned an error: %v", err)
	}

	values, err := url.ParseQuery(params)
	if err != nil {
		t.Fatalf("failed to parse actor facet params: %v", err)
	}
	var filters [][]string
	if err := json.Unmarshal([]byte(values.Get("facetFilters")), &filters); err != nil {
		t.Fatalf("failed to decode actor facet filters: %v", err)
	}
	if want := [][]string{{"actors.actor_id:122245"}}; !reflect.DeepEqual(filters, want) {
		t.Fatalf("actor facet filters = %#v, want %#v", filters, want)
	}
}

func TestPlayboyPlusSearchHitUsesAlgoliaFieldNames(t *testing.T) {
	var hit playboyPlusSearchHit
	err := json.Unmarshal([]byte(`{
		"objectID":"177212-870-870",
		"title":"Kate Jones in Exquisite Allure",
		"url_title":"Kate-Jones-in-Exquisite-Allure",
		"date_online":"2019-03-20",
		"picture":"/177212/previews/177212_1.jpg",
		"actors":[{"name":"Kate Jones"}]
	}`), &hit)
	if err != nil {
		t.Fatalf("failed to decode photoset hit: %v", err)
	}
	if hit.URLTitle != "Kate-Jones-in-Exquisite-Allure" ||
		hit.ReleaseDate != "2019-03-20" ||
		hit.Thumbnail != "/177212/previews/177212_1.jpg" ||
		len(hit.Actors) != 1 || hit.Actors[0].Name != "Kate Jones" {
		t.Fatalf("unexpected decoded photoset hit: %#v", hit)
	}
	if got := absolutePlayboyPlusURL(hit.Thumbnail); got != "https://www.playboyplus.com/177212/previews/177212_1.jpg" {
		t.Fatalf("absolute thumbnail URL = %q", got)
	}
}
