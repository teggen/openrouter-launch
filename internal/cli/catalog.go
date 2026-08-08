package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/teggen/openrouter-launch/internal/launch"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// loadCatalog returns the model catalog, writing to warnings when stale data
// is served because a refresh failed. Callers pass cmd.ErrOrStderr() so the
// warning honors cobra's IO redirection like every other CLI diagnostic.
//
// This is a shim: Task 7 removes it once resolveAndRun calls the planner,
// which returns the same warning as a value.
func loadCatalog(ctx context.Context, svc *launch.Service, refresh bool, warnings io.Writer) (openrouter.Snapshot, error) {
	snap, err := svc.Snapshot(ctx, refresh)
	if err != nil {
		return openrouter.Snapshot{}, err
	}
	if w, ok := launch.StaleWarning(snap, time.Now()); ok {
		fmt.Fprintf(warnings, "warning: %s\n", w.Message)
	}
	return snap, nil
}
