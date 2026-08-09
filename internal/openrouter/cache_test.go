package openrouter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type stubCatalog struct {
	models []Model
	err    error
	calls  int
}

func (s *stubCatalog) Models(ctx context.Context) ([]Model, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.models, nil
}

func testModels() []Model {
	return []Model{{ID: "anthropic/claude-opus-4.6", Provider: "anthropic"}}
}

func newTestCache(t *testing.T, src Catalog, now time.Time) *Cache {
	t.Helper()
	return &Cache{
		Path:   filepath.Join(t.TempDir(), "models.json"),
		TTL:    time.Hour,
		Source: src,
		Now:    func() time.Time { return now },
	}
}

func TestCacheFetchesWhenEmpty(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	c := newTestCache(t, src, time.Unix(1000, 0))

	snap, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(snap.Models) != 1 {
		t.Fatalf("got %d models, want 1", len(snap.Models))
	}
	if snap.Stale {
		t.Error("fresh fetch marked stale")
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1", src.calls)
	}
	if _, err := os.Stat(c.Path); err != nil {
		t.Errorf("cache file not written: %v", err)
	}
}

// TestCacheWriteKeepsFileAndDirPrivate pins least privilege on write site
// #1. Nothing secret is in the catalog cache, which is exactly why it had
// drifted to 0644/0755 — but nothing outside this process reads it either,
// so there is no cost to closing it. Asserting the group/other bits rather
// than an exact mode keeps the test honest under a stricter umask; under the
// usual 0022 the modes are exactly 0600 and 0700.
func TestCacheWriteKeepsFileAndDirPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	c := newTestCache(t, &stubCatalog{models: testModels()}, time.Unix(1000, 0))
	c.Path = filepath.Join(filepath.Dir(c.Path), "cachedir", "models.json")

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("Load: %v", err)
	}
	info, err := os.Stat(c.Path)
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("cache file mode = %o, want no group or other access (0600)", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(c.Path))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("cache dir mode = %o, want no group or other access (0700)", perm)
	}
}

func TestCacheServesFreshWithoutFetching(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	now := time.Unix(1000, 0)
	c := newTestCache(t, src, now)

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("first Load: %v", err)
	}

	c.Now = func() time.Time { return now.Add(30 * time.Minute) }
	snap, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1 (cache should serve)", src.calls)
	}
	if len(snap.Models) != 1 {
		t.Errorf("got %d models, want 1", len(snap.Models))
	}
}

func TestCacheRefetchesWhenExpired(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	now := time.Unix(1000, 0)
	c := newTestCache(t, src, now)

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("first Load: %v", err)
	}

	c.Now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if src.calls != 2 {
		t.Errorf("source called %d times, want 2 (cache should be expired)", src.calls)
	}
}

func TestCacheForceRefreshIgnoresFreshCache(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	c := newTestCache(t, src, time.Unix(1000, 0))

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := c.Load(context.Background(), true); err != nil {
		t.Fatalf("forced Load: %v", err)
	}
	if src.calls != 2 {
		t.Errorf("source called %d times, want 2", src.calls)
	}
}

func TestCacheFallsBackToStaleOnFetchError(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	now := time.Unix(1000, 0)
	c := newTestCache(t, src, now)

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("first Load: %v", err)
	}

	src.err = errors.New("network down")
	c.Now = func() time.Time { return now.Add(2 * time.Hour) }

	snap, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("Load should succeed using stale cache, got %v", err)
	}
	if !snap.Stale {
		t.Error("snapshot not marked stale")
	}
	if snap.StaleErr == nil {
		t.Error("StaleErr not populated")
	}
	if len(snap.Models) != 1 {
		t.Errorf("got %d models, want 1 from stale cache", len(snap.Models))
	}
}

func TestCacheFailsWhenNoCacheAndFetchFails(t *testing.T) {
	src := &stubCatalog{err: errors.New("network down")}
	c := newTestCache(t, src, time.Unix(1000, 0))

	if _, err := c.Load(context.Background(), false); err == nil {
		t.Fatal("expected error when fetch fails with no cache")
	}
}

func TestCacheRecoversFromCorruptFile(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	c := newTestCache(t, src, time.Unix(1000, 0))

	if err := os.WriteFile(c.Path, []byte("{{{corrupt"), 0o644); err != nil {
		t.Fatalf("write corrupt cache: %v", err)
	}

	snap, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("Load should recover from corrupt cache: %v", err)
	}
	if len(snap.Models) != 1 {
		t.Errorf("got %d models, want 1", len(snap.Models))
	}
}

func TestCachePathUsesXDGCacheHome(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdgDir)

	path, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}

	expected := filepath.Join(xdgDir, "openrouter-launch", "models.json")
	if path != expected {
		t.Errorf("got %s, want %s", path, expected)
	}
}

func TestCachePathFallsBackToHomeCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")

	path, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	expected := filepath.Join(home, ".cache", "openrouter-launch", "models.json")
	if path != expected {
		t.Errorf("got %s, want %s", path, expected)
	}
}
