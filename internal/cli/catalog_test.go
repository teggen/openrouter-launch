package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// erroringCatalog always fails, forcing loadCatalog down the stale-cache
// fallback path so its warning behavior can be exercised.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return nil, errors.New("network down")
}

// writeCacheFileForTest writes a catalog cache file in the on-disk shape
// openrouter.Cache expects, without depending on its unexported cacheFile
// type: only the JSON shape needs to match.
func writeCacheFileForTest(t *testing.T, path string, fetchedAt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(struct {
		FetchedAt time.Time          `json:"fetched_at"`
		Models    []openrouter.Model `json:"models"`
	}{
		FetchedAt: fetchedAt,
		Models:    []openrouter.Model{{ID: "anthropic/claude-opus-4.6"}},
	})
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// cachePathForTest isolates the catalog cache to a temp dir and returns its
// resolved path.
func cachePathForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	return path
}

func TestLoadCatalogWarnsOnProvidedWriterWhenStale(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour)) // older than DefaultTTL

	prev := catalogSource
	catalogSource = erroringCatalog{}
	t.Cleanup(func() { catalogSource = prev })

	var warnings bytes.Buffer
	snap, err := loadCatalog(context.Background(), false, &warnings)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if !snap.Stale {
		t.Fatal("expected a stale snapshot")
	}
	if warnings.Len() == 0 {
		t.Error("expected a warning written to the provided writer")
	}
	if !strings.Contains(warnings.String(), "warning:") {
		t.Errorf("warning text = %q, want it to mention the stale refresh", warnings.String())
	}
}

func TestLoadCatalogWritesNothingWhenFresh(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now()) // well within DefaultTTL

	prev := catalogSource
	catalogSource = erroringCatalog{} // must not even be consulted for a fresh cache
	t.Cleanup(func() { catalogSource = prev })

	var warnings bytes.Buffer
	snap, err := loadCatalog(context.Background(), false, &warnings)
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if snap.Stale {
		t.Fatal("fresh cache reported as stale")
	}
	if warnings.Len() != 0 {
		t.Errorf("expected no warning written for fresh data, got %q", warnings.String())
	}
}
