package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

type stubCatalog struct {
	models []catalog.Model
	err    error
	calls  int
}

func (s *stubCatalog) Models(ctx context.Context) ([]catalog.Model, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.models, nil
}

func testModels() []catalog.Model {
	return []catalog.Model{{ID: "anthropic/claude-opus-4.6", Provider: "anthropic"}}
}

func newTestCache(t *testing.T, src catalog.Catalog, now time.Time) *Cache {
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

// TestCacheTreatsAnOlderSchemaAsAMiss is the guard on the one failure mode
// catalog.Model's missing json tags create. A file from a previous format
// decodes WITHOUT error — encoding/json ignores what it cannot match and
// leaves the rest zero — so a renamed or retagged price field would produce a
// full catalog of $0.00 models rather than a decode failure. Free is a claim;
// this is what stops the tool making it. See CacheSchema.
func TestCacheTreatsAnOlderSchemaAsAMiss(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	now := time.Unix(1000, 0)
	c := newTestCache(t, src, now)

	// The pre-schema on-disk shape, fresh by every other measure.
	legacy := fmt.Sprintf(`{"fetched_at":%q,"models":[{"ID":"a/b","PromptPricePerM":15}]}`,
		now.Format(time.RFC3339Nano))
	if err := os.WriteFile(c.Path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}

	snap, err := c.Load(context.Background(), false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1: a file with no schema must be a miss", src.calls)
	}
	if len(snap.Models) != 1 || snap.Models[0].ID != testModels()[0].ID {
		t.Errorf("Load served %v, want the freshly fetched models", snap.Models)
	}
}

// TestCacheRoundTripsWhatItWrites is the other half: the schema check must
// reject only OTHER versions, not everything. Without this, a check that
// rejected every file would pass the test above while disabling the cache.
func TestCacheRoundTripsWhatItWrites(t *testing.T) {
	src := &stubCatalog{models: testModels()}
	now := time.Unix(1000, 0)
	c := newTestCache(t, src, now)

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	var onDisk struct {
		Schema int `json:"schema"`
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	if onDisk.Schema != CacheSchema {
		t.Errorf("written schema = %d, want %d", onDisk.Schema, CacheSchema)
	}

	if _, err := c.Load(context.Background(), false); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1: the cache did not serve its own file", src.calls)
	}
}

// TestSnapshotterReadsThroughTheCache pins the composition internal/launch no
// longer performs: source behind Cache, at CachePath, fresh for DefaultTTL.
// Those are three separate facts about this tool, and the planner used to
// name all three.
func TestSnapshotterReadsThroughTheCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	src := &stubCatalog{models: testModels()}

	load := Snapshotter(src)
	snap, err := load(context.Background(), false)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if len(snap.Models) != len(testModels()) {
		t.Errorf("got %d models, want %d", len(snap.Models), len(testModels()))
	}

	// The file landed where CachePath says, which is what makes the second
	// load a cache hit rather than a second fetch.
	path, err := CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Snapshotter did not write the cache at CachePath: %v", err)
	}
	if _, err := load(context.Background(), false); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1: the loader did not read through the cache", src.calls)
	}
}

// TestSnapshotterHonorsRefresh is the other half: --refresh has to reach the
// cache, or a user who asks for fresh data silently gets yesterday's.
func TestSnapshotterHonorsRefresh(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	src := &stubCatalog{models: testModels()}

	load := Snapshotter(src)
	if _, err := load(context.Background(), false); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, err := load(context.Background(), true); err != nil {
		t.Fatalf("refresh load: %v", err)
	}
	if src.calls != 2 {
		t.Errorf("source called %d times, want 2: refresh did not bypass the fresh cache", src.calls)
	}
}

// TestSnapshotterResolvesCachePathPerCall pins the choice not to resolve it
// once at construction. XDG_CACHE_HOME changes between calls in every test
// that isolates the cache, and a path captured at construction would send
// those writes to whichever directory happened to be set first.
func TestSnapshotterResolvesCachePathPerCall(t *testing.T) {
	load := Snapshotter(&stubCatalog{models: testModels()})

	first := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", first)
	if _, err := load(context.Background(), false); err != nil {
		t.Fatalf("first load: %v", err)
	}

	second := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", second)
	if _, err := load(context.Background(), false); err != nil {
		t.Fatalf("second load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second, "openrouter-launch", "models.json")); err != nil {
		t.Errorf("the second load did not write under the current XDG_CACHE_HOME: %v", err)
	}
}

// TestSnapshotterDefaultsToTheLiveClient covers the nil-source branch without
// touching the network: a fresh cache short-circuits the fetch, so the client
// is constructed and never dialled.
func TestSnapshotterDefaultsToTheLiveClient(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// Seed a fresh cache through the same code path, then hand off to a
	// loader with no source of its own.
	if _, err := Snapshotter(&stubCatalog{models: testModels()})(context.Background(), false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	snap, err := Snapshotter(nil)(context.Background(), false)
	if err != nil {
		t.Fatalf("Snapshotter(nil): %v", err)
	}
	if len(snap.Models) != len(testModels()) {
		t.Errorf("got %d models, want the cached %d", len(snap.Models), len(testModels()))
	}
}
