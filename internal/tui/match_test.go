package tui

import (
	"testing"

	"github.com/teggen/agentlaunch/catalog"
)

func ids(models []catalog.Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ID
	}
	return out
}

func equal(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRankEmptyQueryPreservesCatalogOrder(t *testing.T) {
	in := []catalog.Model{{ID: "z/one"}, {ID: "a/two"}, {ID: "m/three"}}
	if got := ids(Rank(in, "")); !equal(got, "z/one", "a/two", "m/three") {
		t.Errorf("Rank(_, \"\") = %v, want catalog order", got)
	}
}

func TestRankEmptyQueryDoesNotAliasTheInput(t *testing.T) {
	in := []catalog.Model{{ID: "z/one"}, {ID: "a/two"}}
	out := Rank(in, "")
	out[0] = catalog.Model{ID: "mutated"}
	if in[0].ID != "z/one" {
		t.Errorf("Rank returned a slice aliasing its input; caller mutation reached the catalog")
	}
}

// The fixture lists the longer ID first, so a Rank that returned catalog
// order would fail.
func TestRankExactMatchBeatsPrefixMatch(t *testing.T) {
	in := []catalog.Model{{ID: "a/bc"}, {ID: "a/b"}}
	if got := ids(Rank(in, "a/b")); !equal(got, "a/b", "a/bc") {
		t.Errorf("Rank = %v, want the exact match first", got)
	}
}

func TestRankPrefixBeatsSubstring(t *testing.T) {
	in := []catalog.Model{{ID: "z/openai-mirror"}, {ID: "openai/o1"}}
	if got := ids(Rank(in, "openai")); !equal(got, "openai/o1", "z/openai-mirror") {
		t.Errorf("Rank = %v, want the prefix match first", got)
	}
}

func TestRankIDMatchBeatsNameMatch(t *testing.T) {
	in := []catalog.Model{
		{ID: "z/other", Name: "Claude flavored"},
		{ID: "anthropic/claude", Name: "Something else"},
	}
	if got := ids(Rank(in, "claude")); !equal(got, "anthropic/claude", "z/other") {
		t.Errorf("Rank = %v, want the ID match first", got)
	}
}

func TestRankEarlierMatchPositionWins(t *testing.T) {
	// Neither is a prefix match, so both score as ID substrings and only the
	// match offset separates them.
	in := []catalog.Model{{ID: "zzz/mini-x"}, {ID: "a/mini"}}
	if got := ids(Rank(in, "mini")); !equal(got, "a/mini", "zzz/mini-x") {
		t.Errorf("Rank = %v, want the earlier match first", got)
	}
}

func TestRankShorterIDWinsAtTheSamePosition(t *testing.T) {
	in := []catalog.Model{{ID: "mini-extra-long"}, {ID: "mini-x"}}
	if got := ids(Rank(in, "mini")); !equal(got, "mini-x", "mini-extra-long") {
		t.Errorf("Rank = %v, want the shorter ID first", got)
	}
}

func TestRankExcludesNonMatches(t *testing.T) {
	in := []catalog.Model{{ID: "a/one"}, {ID: "b/two"}}
	if got := Rank(in, "nothing-matches-this"); len(got) != 0 {
		t.Errorf("Rank = %v, want no results", ids(got))
	}
}

func TestRankIsCaseInsensitive(t *testing.T) {
	in := []catalog.Model{{ID: "anthropic/claude-opus", Name: "Anthropic: Claude"}}
	if got := Rank(in, "ANTHROPIC"); len(got) != 1 {
		t.Errorf("Rank(%q) matched %d models, want 1", "ANTHROPIC", len(got))
	}
}

// The spec scopes search to slug and display name. Description is decoded and
// shown in the detail pane but is deliberately not searched: ollama's selector
// does rank on description, so this is the one place this implementation
// diverges from the source it is modelled on, and it needs a test to stay
// diverged.
func TestRankDoesNotSearchDescriptions(t *testing.T) {
	in := []catalog.Model{
		{ID: "a/one", Name: "One", Description: "excellent at haskell"},
	}
	if got := Rank(in, "haskell"); len(got) != 0 {
		t.Errorf("Rank matched on Description; want slug and name only")
	}
}

// Documentation of intent: equal scores keep catalog order. sort.Slice is not
// guaranteed to reorder equal elements, so this cannot reliably fail if
// SliceStable were swapped for Slice — it records the contract rather than
// enforcing it.
func TestRankKeepsCatalogOrderForEqualScores(t *testing.T) {
	in := []catalog.Model{{ID: "a/mini"}, {ID: "b/mini"}}
	if got := ids(Rank(in, "mini")); !equal(got, "a/mini", "b/mini") {
		t.Errorf("Rank = %v, want catalog order preserved for ties", got)
	}
}
