// Package provider holds this tool's identity: the one OpenRouter descriptor
// every launcher is bound to, and the host name it presents itself under.
//
// It is deliberately tiny and deliberately separate. github.com/teggen/agentlaunch
// ships the eleven launch recipes knowing nothing about any vendor, so
// naming one is this repository's job — and keeping it in a package of its
// own, rather than inline at the composition root, is what lets the values be
// asserted directly by the tests that pin them.
package provider

import "github.com/teggen/agentlaunch/agent"

// OpenRouter is the provider this tool was built for.
//
// This value is the whole OpenRouter-specific surface of the launch path:
// every launcher reads its endpoints, its key variable and its ID rather than
// a constant of its own. It lived in the agent package until the extraction,
// as a way station while the registry still defaulted to a provider instead
// of taking one.
var OpenRouter = agent.Provider{
	ID:          "openrouter",
	DisplayName: "OpenRouter",

	BaseURL: "https://openrouter.ai/api/v1",
	// No /v1 — Claude Code appends its own version segment. See the field
	// comment on agent.Provider.AnthropicBaseURL, and Landmine 1.
	AnthropicBaseURL: "https://openrouter.ai/api",

	APIKeyEnv:      "OPENROUTER_API_KEY",
	RequiresAPIKey: true,
	KeysURL:        "https://openrouter.ai/keys",

	ModelPrefix: "openrouter/",
	// Live-verified against codex 0.146.1: "chat" is rejected at config-load
	// time. Landmine 18.
	WireAPI: "responses",
}

// OpenRouterHost is this tool's identity.
//
// Marker must stay "openrouter-launch" for as long as this tool exists,
// whatever the binary is called: it is already written into users'
// ~/.factory/settings.local.json files, and droid's restore recognises our
// entries by exact string match. See agent.Host.Marker.
var OpenRouterHost = agent.Host{
	Name:   "openrouter-launch",
	Marker: "openrouter-launch",
}

// Registry binds every builtin launch recipe to OpenRouter.
//
// This is the composition root's one call, and the only place in this tool
// where a provider is named. MustRegistry panics rather than returning an
// error on purpose: a malformed registry is a programming error in this
// file, not a condition a user can be asked to resolve, and it must surface
// at startup rather than at the first launch attempt.
func Registry() *agent.Registry {
	return agent.MustRegistry(agent.Binding{Provider: OpenRouter, Host: OpenRouterHost}, agent.Builtins())
}
