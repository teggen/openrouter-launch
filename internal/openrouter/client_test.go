package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestClientModels(t *testing.T) {
	fixture, err := os.ReadFile("testdata/models.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}

	if len(models) != 3 {
		t.Errorf("got %d models, want 3", len(models))
	}
	if gotPath != "/models" {
		t.Errorf("path = %q, want /models", gotPath)
	}
	if gotQuery != "sort=most-popular" {
		t.Errorf("query = %q, want sort=most-popular", gotQuery)
	}
}

func TestClientModelsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Models(context.Background()); err == nil {
		t.Fatal("expected an error for HTTP 500")
	}
}

func TestClientSatisfiesCatalog(t *testing.T) {
	var _ Catalog = NewClient()
}

func TestNewClientUsesDefaultBaseURL(t *testing.T) {
	if got := NewClient().BaseURL; got != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", got, DefaultBaseURL)
	}
}
