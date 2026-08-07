package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// catalogSource overrides the HTTP client in tests. nil means use the real one.
var catalogSource openrouter.Catalog

// loadCatalog returns the model catalog, warning on stderr when stale data is
// served because a refresh failed.
func loadCatalog(ctx context.Context, refresh bool) (openrouter.Snapshot, error) {
	path, err := openrouter.CachePath()
	if err != nil {
		return openrouter.Snapshot{}, err
	}

	source := catalogSource
	if source == nil {
		source = openrouter.NewClient()
	}

	cache := &openrouter.Cache{Path: path, TTL: openrouter.DefaultTTL, Source: source}
	snap, err := cache.Load(ctx, refresh)
	if err != nil {
		return openrouter.Snapshot{}, fmt.Errorf("load model catalog: %w", err)
	}

	if snap.Stale {
		fmt.Fprintf(os.Stderr,
			"warning: could not refresh the model catalog (%v); using cached data from %s ago\n",
			snap.StaleErr, snap.Age(time.Now()).Round(time.Minute))
	}
	return snap, nil
}
