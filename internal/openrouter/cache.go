package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultTTL is how long a cached catalog is considered fresh.
const DefaultTTL = 24 * time.Hour

// Snapshot is a catalog read plus its provenance.
type Snapshot struct {
	Models    []Model
	FetchedAt time.Time
	// Stale is true when a refresh failed and cached data was served instead.
	Stale bool
	// StaleErr is the refresh failure that caused Stale.
	StaleErr error
}

// Age reports how old the snapshot's data is relative to now.
func (s Snapshot) Age(now time.Time) time.Duration {
	return now.Sub(s.FetchedAt)
}

// Cache reads the catalog through Source, persisting it to Path.
type Cache struct {
	Path   string
	TTL    time.Duration
	Source Catalog
	// Now is injectable for tests; nil means time.Now.
	Now func() time.Time
}

type cacheFile struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []Model   `json:"models"`
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Load returns the catalog, fetching when the cache is missing, expired, or
// forceRefresh is set. If a fetch fails but cached data exists, the cached
// data is returned with Stale set rather than failing.
func (c *Cache) Load(ctx context.Context, forceRefresh bool) (Snapshot, error) {
	cached, hasCache := c.read()

	if !forceRefresh && hasCache && c.now().Sub(cached.FetchedAt) < c.TTL {
		return Snapshot{Models: cached.Models, FetchedAt: cached.FetchedAt}, nil
	}

	models, err := c.Source.Models(ctx)
	if err != nil {
		if hasCache {
			return Snapshot{
				Models:    cached.Models,
				FetchedAt: cached.FetchedAt,
				Stale:     true,
				StaleErr:  err,
			}, nil
		}
		return Snapshot{}, err
	}

	fetchedAt := c.now()
	c.write(cacheFile{FetchedAt: fetchedAt, Models: models})
	return Snapshot{Models: models, FetchedAt: fetchedAt}, nil
}

// read returns the cached file, reporting false when it is missing or corrupt.
func (c *Cache) read() (cacheFile, bool) {
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return cacheFile{}, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return cacheFile{}, false
	}
	// An empty Models slice is also treated as a miss: it guards against a
	// truncated or otherwise empty write. Worst case this just forces a
	// refetch, which is harmless.
	if len(cf.Models) == 0 {
		return cacheFile{}, false
	}
	return cf, true
}

// write persists the cache. Failures are ignored: a cache miss is recoverable
// and must never block a launch.
func (c *Cache) write(cf cacheFile) {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.Path, data, 0o644)
}

// CachePath returns the on-disk catalog cache location.
func CachePath() (string, error) {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve cache dir: %w", err)
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "openrouter-launch", "models.json"), nil
}
