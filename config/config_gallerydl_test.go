package config

import "testing"

func TestGetEnvAsBool(t *testing.T) {
	t.Setenv("GALLERYDL_TEST_BOOL", "true")
	if !getEnvAsBool("GALLERYDL_TEST_BOOL", false) {
		t.Fatal("true should enable boolean environment setting")
	}

	t.Setenv("GALLERYDL_TEST_BOOL", "off")
	if getEnvAsBool("GALLERYDL_TEST_BOOL", true) {
		t.Fatal("off should disable boolean environment setting")
	}

	t.Setenv("GALLERYDL_TEST_BOOL", "")
	if getEnvAsBool("GALLERYDL_TEST_BOOL", true) != true {
		t.Fatal("an empty setting should preserve the fallback")
	}
}

func TestLoadPersonScanWorkers(t *testing.T) {
	t.Setenv("PERSON_SCAN_WORKERS", "3")
	Load()
	if Global.PersonScanWorkers != 3 {
		t.Fatalf("PersonScanWorkers = %d, want 3", Global.PersonScanWorkers)
	}

	t.Setenv("PERSON_SCAN_WORKERS", "0")
	Load()
	if Global.PersonScanWorkers != 1 {
		t.Fatalf("PersonScanWorkers = %d, want minimum 1", Global.PersonScanWorkers)
	}
}
