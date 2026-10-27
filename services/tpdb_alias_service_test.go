package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTPDBAliasServiceSearchReturnsSiteAliases(t *testing.T) {
	apiKey := "unit-test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/performers" || r.URL.Query().Get("q") != "Jane Doe" {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Fatal("TPDB authorization header was not set")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"performer-id","name":"Jane Doe","site_performers":[{"name":"Jane D","site":{"name":"MetArt X"}},{"name":"Jane D","site":{"name":"Unsupported Studio"}},{"name":"","site":{"name":"MetArt"}},{"name":"Missing Site"}]}]}`))
	}))
	defer server.Close()

	service := NewTPDBAliasService(apiKey)
	service.baseURL = server.URL

	results, err := service.Search(context.Background(), "Jane Doe")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "performer-id" {
		t.Fatalf("unexpected performer results: %#v", results)
	}
	if len(results[0].Aliases) != 1 || results[0].Aliases[0] != (TPDBSiteAlias{
		Provider: "MetartX",
		Alias:    "Jane D",
	}) {
		t.Fatalf("unexpected site aliases: %#v", results[0].Aliases)
	}
}

func TestCanonicalTPDBScraperProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
		ok       bool
	}{
		{name: "known provider case and spacing", provider: "Met Art X", want: "MetartX", ok: true},
		{name: "Playboy maps to supported scanner", provider: "Playboy", want: "PlayboyPlus", ok: true},
		{name: "unsupported studio", provider: "Unsupported Studio", ok: false},
		{name: "Vixen has no missing-gallery scraper", provider: "Vixen", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := canonicalTPDBScraperProvider(test.provider)
			if got != test.want || ok != test.ok {
				t.Fatalf("canonicalTPDBScraperProvider(%q) = (%q, %t), want (%q, %t)",
					test.provider, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestTPDBAliasServiceRequiresAPIKey(t *testing.T) {
	service := NewTPDBAliasService("")
	if _, err := service.Search(context.Background(), "Jane Doe"); err == nil {
		t.Fatal("expected missing API key to return an error")
	}
}
