package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnqueueSourceRequestsLiveCrawl(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		if r.Method != http.MethodPost || r.URL.Path != "/api/sources/42/crawl" {
			t.Errorf("request = %s %s, want POST /api/sources/42/crawl", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	if err := enqueueSource(client, server.URL+"/api/", 42); err != nil {
		t.Fatal(err)
	}
	if !requested {
		t.Fatal("expected live crawl endpoint to be called")
	}
}

func TestEnqueueSourceReportsServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	if err := enqueueSource(client, server.URL, 42); err == nil {
		t.Fatal("expected non-success response to return an error")
	}
}
