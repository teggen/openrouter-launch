package provider

import (
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/agent"
)

func TestOpenRouterProviderAndHostValidate(t *testing.T) {
	if err := OpenRouter.Validate(); err != nil {
		t.Fatalf("OpenRouter.Validate() = %v, want nil", err)
	}
	if err := OpenRouterHost.Validate(); err != nil {
		t.Fatalf("OpenRouterHost.Validate() = %v, want nil", err)
	}
}

// TestOpenRouterBaseURLsDifferByTheVersionSegment is Landmine 1 stated as the
// relationship it actually is, rather than as two facts about two constants.
// The OpenAI-compatible root carries a version segment because clients on
// that protocol append only a method path; the Anthropic root must not,
// because Claude Code appends its own. Unifying them — which is correct for
// providers whose two roots really are the same string — yields
// /api/v1/v1/messages here.
//
// agent.Provider.Validate enforces the general half of this rule (no version
// segment on an Anthropic root, for any provider). It cannot enforce the
// half below, that OpenRouter's two roots differ by exactly "/v1", because
// only this repository knows what OpenRouter's roots are.
func TestOpenRouterBaseURLsDifferByTheVersionSegment(t *testing.T) {
	if !strings.HasSuffix(OpenRouter.BaseURL, "/v1") {
		t.Errorf("BaseURL = %q, want a /v1 suffix", OpenRouter.BaseURL)
	}
	if strings.HasSuffix(OpenRouter.AnthropicBaseURL, "/v1") {
		t.Errorf("AnthropicBaseURL = %q must not end in /v1", OpenRouter.AnthropicBaseURL)
	}
	if OpenRouter.AnthropicBaseURL+"/v1" != OpenRouter.BaseURL {
		t.Errorf("the two roots must differ by exactly the version segment: %q + /v1 != %q",
			OpenRouter.AnthropicBaseURL, OpenRouter.BaseURL)
	}
}

// TestOpenRouterHostMarkerIsFrozen guards persisted user data, not a label.
//
// droid's Apply writes a customModels entry whose displayName is this marker,
// points droid's default model at "custom:<marker>-<n>", and on restore
// removes exactly the entries that carry it. Any other value makes every
// entry a previous version of this tool wrote unrecognisable — preserved as
// somebody else's forever, alongside a default-model reference to an entry
// that no longer exists. Renaming the binary does not license renaming this,
// and neither did moving the launchers into another module.
func TestOpenRouterHostMarkerIsFrozen(t *testing.T) {
	if OpenRouterHost.Marker != "openrouter-launch" {
		t.Errorf("OpenRouterHost.Marker = %q; it is written into users' "+
			"~/.factory/settings.local.json and must not change",
			OpenRouterHost.Marker)
	}
}

// TestRegistryIsTheOneBindingThisToolShips pins that Registry() binds the
// values above rather than a second copy of them. The golden launch surface
// is asserted against whatever this function returns, so a binding that
// quietly named a different provider or host would carry every expectation in
// goldenlaunch_test.go with it and none of them would fail.
func TestRegistryIsTheOneBindingThisToolShips(t *testing.T) {
	spec, err := Registry().Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup(claude): %v", err)
	}
	c, ok := spec.Launcher.(*agent.Claude)
	if !ok {
		t.Fatalf("claude's launcher is %T, want *agent.Claude", spec.Launcher)
	}
	if c.Provider.ID != OpenRouter.ID {
		t.Errorf("claude is bound to provider %q, want %q", c.Provider.ID, OpenRouter.ID)
	}
	if c.Host.Marker != OpenRouterHost.Marker {
		t.Errorf("claude is bound to host marker %q, want %q", c.Host.Marker, OpenRouterHost.Marker)
	}
}
