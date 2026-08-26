package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// DefaultTTL is how long a cached catalog is considered fresh.
const DefaultTTL = 24 * time.Hour

// Cache reads the catalog through Source, persisting it to Path.
type Cache struct {
	Path   string
	TTL    time.Duration
	Source catalog.Catalog
	// Now is injectable for tests; nil means time.Now.
	Now func() time.Time
}

// CacheSchema is the on-disk format version. Bump it whenever catalog.Model
// changes in a way that makes an older file decode WRONGLY rather than fail
// to decode — a renamed or retyped field, or the addition of json tags.
//
// This exists because catalog.Model carries no json tags, so the file stores
// Go field names ("PromptPricePerM"). encoding/json matches names
// case-insensitively and silently zeroes what it cannot match, so any such
// change leaves every old file decoding cleanly with a zero price — and a
// zero price is not "unknown", it is FREE. A $75/M model rendered as free is
// Landmine 4's exact false claim, reached without anyone writing a wrong
// price anywhere. A version mismatch is treated as a miss, which costs one
// refetch of a public endpoint.
//
// It is exported because three packages' tests seed a cache file by writing
// the JSON shape directly rather than depending on the unexported cacheFile
// type; hardcoding the number in each of them is the drift those helpers
// already exist to avoid.
const CacheSchema = 1

type cacheFile struct {
	// Schema is absent from files written before it existed, so those decode
	// as 0 and are correctly treated as a miss.
	Schema    int             `json:"schema"`
	FetchedAt time.Time       `json:"fetched_at"`
	Models    []catalog.Model `json:"models"`
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
func (c *Cache) Load(ctx context.Context, forceRefresh bool) (catalog.Snapshot, error) {
	cached, hasCache := c.read()

	if !forceRefresh && hasCache && c.now().Sub(cached.FetchedAt) < c.TTL {
		return catalog.Snapshot{Models: cached.Models, FetchedAt: cached.FetchedAt}, nil
	}

	models, err := c.Source.Models(ctx)
	if err != nil {
		if hasCache {
			return catalog.Snapshot{
				Models:    cached.Models,
				FetchedAt: cached.FetchedAt,
				Stale:     true,
				StaleErr:  err,
			}, nil
		}
		return catalog.Snapshot{}, err
	}

	fetchedAt := c.now()
	c.write(cacheFile{Schema: CacheSchema, FetchedAt: fetchedAt, Models: models})
	return catalog.Snapshot{Models: models, FetchedAt: fetchedAt}, nil
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
	// A file written by a different schema decodes without error but cannot
	// be trusted field-for-field; see CacheSchema.
	if cf.Schema != CacheSchema {
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
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	// 0600/0700 despite holding nothing secret — the catalog is public. This
	// process is the only reader, so the broader modes bought nothing.
	_ = os.WriteFile(c.Path, data, 0o600)
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
