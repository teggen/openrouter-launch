package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/teggen/agentlaunch/agent"
	"github.com/teggen/agentlaunch/catalog"
)

// This file is the other half of the provider parameterization, and after the
// extraction it is the half that had to stay behind.
//
// Every launcher test in github.com/teggen/agentlaunch runs against a
// synthetic provider, which is what proves a launcher reads its Provider
// field instead of a constant — but a launcher could read the field correctly
// and still be wired to the WRONG OpenRouter value, and no synthetic-provider
// test can see that. Only the tool that names the provider knows what the
// right values are, so this test lives here: it pins the exact argv and
// environment each agent is launched with against the registry this tool
// actually ships.
//
// It builds that registry through Registry(), not through a binding of its
// own. The distinction matters — a re-derived binding would keep passing if
// the composition root started building a different one.
//
// The values below were captured from the launchers as they stood before the
// provider descriptor existed and verified byte-for-byte against them, so a
// failure here means an observable change to what a user's agent receives —
// which is a major version bump by this project's own semver contract, not a
// refactor. Neither half of the pair is sufficient alone: restoring codex's
// falsified wire_api="chat" (Landmine 18) fails ONLY this test.

// goldenStageDir is the staging directory the golden request names. Its value
// does not matter; that openclaw's config path is derived from it does.
const goldenStageDir = "/tmp/orl-golden-stage"

func openclawPath(*testing.T) string {
	return filepath.Join(goldenStageDir, "openclaw.json")
}

// testHome points every home-directory lookup reached by this test at a fresh
// temp dir, on every platform.
//
// `t.Setenv("HOME", dir)` alone is NOT enough — the gap Landmine 8 did not
// account for. os.UserHomeDir reads HOME on Unix but USERPROFILE on Windows,
// so on Windows the agent code would keep resolving the real user's home, and
// Droid.Apply would write ~/.factory/settings.local.json into the developer's
// actual profile. APPDATA and LOCALAPPDATA are redirected for the same
// reason: Hermes.findPath and Qwen.findPath consult them on Windows, so
// leaving them pointed at the real profile lets a genuinely installed agent
// satisfy a test that needs the binary to be absent.
//
// The agent package keeps its own copy of this helper. A _test.go helper
// cannot be imported across packages, and exporting one from production code
// to serve tests would put it in the module's public API.
func testHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)        // Unix, and anything reading it directly
	t.Setenv("USERPROFILE", dir) // Windows: what os.UserHomeDir actually reads
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	return dir
}

// envValue finds a KEY=VALUE entry in a command's environment. It reports
// presence separately from the value, because an empty value is meaningful
// here: ANTHROPIC_AUTH_TOKEN present-but-empty is what stops Claude Code
// falling back to its own Anthropic authentication (Landmine 2).
func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if len(kv) > len(key) && kv[:len(key)] == key && kv[len(key)] == '=' {
			return kv[len(key)+1:], true
		}
	}
	return "", false
}

func TestOpenRouterLaunchSurfaceIsUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake binaries this plants are not executable on Windows")
	}
	// Landmine 8: real installs exist on this machine, and findPath has
	// home-directory fallbacks, so both HOME and PATH have to be ours.
	testHome(t)
	bin := t.TempDir()
	for _, n := range []string{"claude", "codex", "opencode", "pi", "hermes",
		"qwen", "cline", "kimi", "omp", "openclaw", "droid"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	req := agent.Request{
		Model: catalog.Model{
			ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, SupportsTools: true, Provider: "anthropic",
		},
		APIKey:   "sk-or-test",
		StageDir: goldenStageDir,
	}

	golden := []struct {
		name   string
		args   []string
		env    map[string]string
		staged string
	}{
		{
			name: "claude",
			args: []string{"--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"ANTHROPIC_API_KEY":              "sk-or-test",
				"ANTHROPIC_AUTH_TOKEN":           "",
				"ANTHROPIC_BASE_URL":             "https://openrouter.ai/api",
				"ANTHROPIC_DEFAULT_FABLE_MODEL":  "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_OPUS_MODEL":   "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_SONNET_MODEL": "anthropic/claude-opus-4.6",
				"CLAUDE_CODE_SUBAGENT_MODEL":     "anthropic/claude-opus-4.6",
			},
		},
		{
			name: "codex",
			args: []string{"-c", "model_provider=\"openrouter\"", "-c", "model_providers.openrouter.name=\"OpenRouter\"", "-c", "model_providers.openrouter.base_url=\"https://openrouter.ai/api/v1\"", "-c", "model_providers.openrouter.env_key=\"OPENROUTER_API_KEY\"", "-c", "model_providers.openrouter.wire_api=\"responses\"", "-m", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "opencode",
			args: []string{},
			env: map[string]string{
				"OPENCODE_CONFIG_CONTENT": "{\"$schema\":\"https://opencode.ai/config.json\",\"model\":\"openrouter/anthropic/claude-opus-4.6\"}",
				"OPENROUTER_API_KEY":      "sk-or-test",
			},
		},
		{
			name: "pi",
			args: []string{"--provider", "openrouter", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "hermes",
			args: []string{"chat", "--provider", "openrouter", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY":  "sk-or-test",
				"OPENROUTER_BASE_URL": "https://openrouter.ai/api/v1",
			},
		},
		{
			name: "qwen",
			args: []string{"--auth-type", "openai", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENAI_API_KEY":     "sk-or-test",
				"OPENAI_BASE_URL":    "https://openrouter.ai/api/v1",
				"OPENAI_MODEL":       "anthropic/claude-opus-4.6",
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "cline",
			args: []string{"-P", "openrouter", "-m", "anthropic/claude-opus-4.6", "-k", "sk-or-test"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "kimi",
			args: []string{},
			env: map[string]string{
				"KIMI_MODEL_API_KEY":          "sk-or-test",
				"KIMI_MODEL_BASE_URL":         "https://openrouter.ai/api/v1",
				"KIMI_MODEL_MAX_CONTEXT_SIZE": "200000",
				"KIMI_MODEL_NAME":             "anthropic/claude-opus-4.6",
				"KIMI_MODEL_PROVIDER_TYPE":    "openai",
			},
		},
		{
			name: "omp",
			args: []string{"--model", "openrouter/anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "openclaw",
			args: []string{"tui", "--local"},
			env: map[string]string{
				"OPENCLAW_CONFIG_PATH": openclawPath(t),
				"OPENROUTER_API_KEY":   "sk-or-test",
			},
			staged: "{\"agents\":{\"defaults\":{\"model\":{\"primary\":\"openrouter/anthropic/claude-opus-4.6\"},\"models\":{\"openrouter/anthropic/claude-opus-4.6\":{}}}}}",
		},
		{
			name: "droid",
			args: []string{},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		}}

	reg := Registry()
	if want := len(golden); want != len(reg.List())-3 {
		t.Fatalf("golden covers %d launchers, registry has %d supported entries",
			want, len(reg.List())-3)
	}

	for _, g := range golden {
		spec, err := reg.Lookup(g.name)
		if err != nil {
			t.Errorf("%s: %v", g.name, err)
			continue
		}
		cmd, err := spec.Launcher.Command(req)
		if err != nil {
			t.Errorf("%s: Command: %v", g.name, err)
			continue
		}
		if !slices.Equal(cmd.Args, g.args) {
			t.Errorf("%s: Args =\n  %q\nwant\n  %q", g.name, cmd.Args, g.args)
		}
		// The env block IS the product: assert the exact set, so a stray or
		// dropped variable fails rather than passing unnoticed.
		if len(cmd.Env) != len(g.env) {
			t.Errorf("%s: len(Env) = %d, want %d (exact set): %q",
				g.name, len(cmd.Env), len(g.env), cmd.Env)
		}
		for k, want := range g.env {
			got, ok := envValue(cmd.Env, k)
			if !ok {
				t.Errorf("%s: %s not set", g.name, k)
				continue
			}
			if got != want {
				t.Errorf("%s: %s = %q, want %q", g.name, k, got, want)
			}
		}
		if g.staged != "" {
			st, ok := spec.Launcher.(agent.Staged)
			if !ok {
				t.Errorf("%s: expected a Staged launcher", g.name)
				continue
			}
			files, err := st.StagedFiles(req)
			if err != nil || len(files) != 1 {
				t.Errorf("%s: StagedFiles = %d files, %v", g.name, len(files), err)
				continue
			}
			if string(files[0].Contents) != g.staged {
				t.Errorf("%s: staged contents =\n  %s\nwant\n  %s",
					g.name, files[0].Contents, g.staged)
			}
		}
	}
}
