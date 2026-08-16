package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientModels(t *testing.T) {
	fixture, err := os.ReadFile("testdata/models.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotPath, gotQuery, gotAuth, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Api-Key")
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
	// The catalog endpoint is public: no credentials should ever be sent.
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty", gotAuth)
	}
	if gotAPIKey != "" {
		t.Errorf("X-Api-Key header = %q, want empty", gotAPIKey)
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

// TestClientModelsRefusesAnOversizedBody pins the read cap. This is the only
// network read in the binary, and io.ReadAll on a response body is bounded by
// nothing the client controls — the HTTP timeout bounds DURATION, so a fast
// endpoint streaming garbage is limited by bandwidth, not by memory.
//
// The cap is set to one byte under the fixture so the test proves the
// boundary rather than some arbitrary large number, and the assertion is on
// the error rather than on len(models): the failure this prevents is a
// SILENT one, where a truncated read decodes into a short catalog that looks
// like a legitimately smaller list.
func TestClientModelsRefusesAnOversizedBody(t *testing.T) {
	fixture, err := os.ReadFile("testdata/models.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), maxBytes: int64(len(fixture)) - 1}
	_, err = c.Models(context.Background())
	if err == nil {
		t.Fatal("expected an error for a body over the cap")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want it to say the response is too large", err)
	}
}

// TestClientModelsAcceptsABodyExactlyAtTheCap is the other half of the
// boundary. A cap implemented as io.LimitReader(body, max) alone cannot tell
// "exactly max bytes" from "truncated at max", so the natural off-by-one is
// to reject a body that is in fact exactly the permitted size. Without this
// test that mistake passes the oversized test above.
func TestClientModelsAcceptsABodyExactlyAtTheCap(t *testing.T) {
	fixture, err := os.ReadFile("testdata/models.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), maxBytes: int64(len(fixture))}
	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models with a body exactly at the cap: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("got %d models, want 3", len(models))
	}
}

func TestNewClientHasAReadCap(t *testing.T) {
	if got := NewClient().maxBytes; got != 0 && got != defaultMaxCatalogBytes {
		t.Errorf("maxBytes = %d, want 0 (meaning the default) or %d", got, defaultMaxCatalogBytes)
	}
	// The default must actually be applied, not merely declared: a zero
	// maxBytes has to resolve to the constant rather than to "no limit".
	c := &Client{}
	if got := c.readLimit(); got != defaultMaxCatalogBytes {
		t.Errorf("readLimit() with an unset maxBytes = %d, want %d", got, defaultMaxCatalogBytes)
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
