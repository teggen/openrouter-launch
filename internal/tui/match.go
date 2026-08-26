// Package tui implements the interactive picker and the launch flow around
// it.
//
// It imports internal/launch and must never import internal/cli: cli imports
// tui, so that edge would be a cycle. See TestTUIDependsOnNeitherCLINorCobra.
//
// Nothing in this package launches an agent. Run returns an approved plan and
// the caller performs the handoff, so every bubbletea program has torn down
// and the terminal is out of raw mode before syscall.Exec replaces the
// process.
package tui

import (
	"sort"
	"strings"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// Rank returns the models matching query, best match first. An empty query
// returns every model in catalog order.
//
// Ranking follows ollama/cmd/tui/selector.go:623: an exact ID match beats an
// ID prefix, which beats an ID substring, which beats a display-name
// substring. Ties break on match position, then on how many runes the field
// has beyond the query, then on catalog order — so a shorter, earlier match
// always wins and the result is deterministic.
//
// Descriptions are not searched. The spec scopes search to slug and display
// name; the description pane exists to explain the highlighted row, not to
// widen the match set.
func Rank(models []catalog.Model, query string) []catalog.Model {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		// Copied rather than returned directly: the picker holds the catalog
		// for the life of the session and reassigns this result on every
		// keystroke, so an aliased slice would let one recompute corrupt the
		// source.
		out := make([]catalog.Model, len(models))
		copy(out, models)
		return out
	}

	type scored struct {
		model catalog.Model
		score matchScore
	}
	hits := make([]scored, 0, len(models))
	for _, m := range models {
		if s := score(m, q); s.ok {
			hits = append(hits, scored{model: m, score: s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return better(hits[i].score, hits[j].score)
	})

	out := make([]catalog.Model, len(hits))
	for i, h := range hits {
		out[i] = h.model
	}
	return out
}

// matchScore is a model's match quality. Lower is better in every field.
type matchScore struct {
	ok bool
	// rank: 0 exact ID, 1 ID prefix, 2 ID substring, 3 name substring.
	rank int
	// index is the rune offset of the match within the matched field.
	index int
	// extra is how many runes the matched field has beyond the query.
	extra int
}

func score(m catalog.Model, q string) matchScore {
	id := strings.ToLower(m.ID)
	name := strings.ToLower(m.Name)

	if id == q {
		return matchScore{ok: true, rank: 0}
	}
	if strings.HasPrefix(id, q) {
		return matchScore{ok: true, rank: 1, extra: runeLen(id) - runeLen(q)}
	}
	if i := strings.Index(id, q); i >= 0 {
		return matchScore{ok: true, rank: 2,
			index: runeLen(id[:i]), extra: runeLen(id) - runeLen(q)}
	}
	if i := strings.Index(name, q); i >= 0 {
		return matchScore{ok: true, rank: 3,
			index: runeLen(name[:i]), extra: runeLen(name) - runeLen(q)}
	}
	return matchScore{}
}

func better(a, b matchScore) bool {
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	if a.index != b.index {
		return a.index < b.index
	}
	return a.extra < b.extra
}

func runeLen(s string) int { return len([]rune(s)) }
