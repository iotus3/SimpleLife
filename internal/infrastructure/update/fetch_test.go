package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/infrastructure/update"
)

func TestLatestReadsTheManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		_, _ = w.Write([]byte(`{"version":" v0.2.0 ","notes":" 模板 ","url":"https://example.com/dl"}`))
	}))
	defer server.Close()

	release, err := update.Fetcher{URL: server.URL}.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if release.Version != "v0.2.0" || release.Notes != "模板" || release.URL != "https://example.com/dl" {
		t.Fatalf("release = %+v", release)
	}
}

func TestLatestWithoutAChannelIsUnsupported(t *testing.T) {
	// A source build has no channel: that is a capability the build lacks, not a
	// failure, and the about page says so instead of showing an error.
	if _, err := (update.Fetcher{}).Latest(context.Background()); !shared.IsKind(err, shared.KindUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
}

func TestLatestReportsABadChannel(t *testing.T) {
	cases := []struct {
		name  string
		serve func(w http.ResponseWriter)
	}{
		{"not-json", func(w http.ResponseWriter) { _, _ = w.Write([]byte("<!doctype html>")) }},
		{"no-version", func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"notes":"x"}`)) }},
		{"500", func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }},
	}
	for _, c := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.serve(w) }))
		_, err := update.Fetcher{URL: server.URL}.Latest(context.Background())
		server.Close()
		if !shared.IsKind(err, shared.KindInvalid) {
			t.Fatalf("%s: want invalid, got %v", c.name, err)
		}
	}
}
