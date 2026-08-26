package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// APIKeyEnvVar is the environment variable that overrides the stored key.
const APIKeyEnvVar = "OPENROUTER_API_KEY"

// ErrNoAPIKey is returned when no key is available from any source.
var ErrNoAPIKey = errors.New("no OpenRouter API key configured")

// ErrUnusableAPIKey is returned when a key IS configured but cannot be used.
//
// Distinct from ErrNoAPIKey because the messages differ — one says "set a
// key", the other has to say which stored value is broken and where it lives
// — but both are recoverable by collecting a new key, and the TUI treats them
// alike for that reason.
var ErrUnusableAPIKey = errors.New("the configured OpenRouter API key cannot be used")

// Filters is the persisted model-picker filter state. Zero numeric values
// mean the filter is unset.
type Filters struct {
	ToolsOnly  bool    `json:"tools_only"`
	FreeOnly   bool    `json:"free_only"`
	MinContext int     `json:"min_context"`
	MaxPrice   float64 `json:"max_price"`
}

// Sort is the persisted models-table ordering. The zero value is "relevance":
// catalog order, or best-match-first while the picker's search box has text.
//
// Column is a plain string rather than an openrouter.SortKey on purpose. This
// package deliberately depends on nothing else in the tree, and an
// unrecognised value must degrade to relevance at the boundary
// (launch.SortFrom) rather than fail a config load — a hand-edited or
// future-version config may not make the listing unusable.
type Sort struct {
	Column string `json:"column,omitempty"`
	Desc   bool   `json:"desc,omitempty"`
}

// Profile is a named agent + model favorite.
type Profile struct {
	Name  string   `json:"name"`
	Agent string   `json:"agent"`
	Model string   `json:"model"`
	Args  []string `json:"args,omitempty"`
}

// Config is the persisted application state.
type Config struct {
	APIKey    string    `json:"api_key,omitempty"`
	Profiles  []Profile `json:"profiles,omitempty"`
	LastAgent string    `json:"last_agent,omitempty"`
	LastModel string    `json:"last_model,omitempty"`
	Filters   Filters   `json:"filters"`
	Sort      Sort      `json:"sort"`
}

// defaults returns the config used when no file exists. Tool calling is on by
// default because a coding agent without it is unusable.
func defaults() *Config {
	return &Config{Filters: Filters{ToolsOnly: true}}
}

// Load reads the config, returning defaults when the file does not exist.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaults(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config at %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config atomically with owner-only permissions, because it
// may contain an API key.
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	// 0700, not 0755: this directory holds the API key file. The file is
	// 0600, so the mode here only governs whether others can see that it
	// exists and when it changed — but on macOS every user shares the
	// `staff` group, so "group" is not a narrower audience than "world".
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "config-*.json")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup. On the success path the rename below has already
	// moved this file, so Remove fails with ENOENT and that failure carries
	// no information; on the error paths the error being returned is the one
	// worth reporting. The explicit `_ =` marks the choice for errcheck.
	defer func() { _ = os.Remove(tmpName) }()

	// Explicit rather than relying on os.CreateTemp's 0600 default: this is
	// the API-key confidentiality guarantee, and this comment is what should
	// stop a future cleanup from deleting the call as redundant.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// ValidateAPIKey reports whether a key can be used, and is the single rule
// this tool applies wherever one arrives — typed at the prompt, read back
// from the config file, or taken from the environment.
//
// A control character is fatal: os/exec refuses an entire environment
// containing a NUL, and a stray carriage return or newline would instead
// travel silently into an Authorization header and come back as a 401 nobody
// can explain. strings.TrimSpace does not catch either, because NUL is not
// whitespace and an embedded CR is not at an end.
//
// The key is never echoed in the error. It is a secret, and one whose defect
// is invisible anyway — the byte and its offset are what a user needs.
func ValidateAPIKey(key string) error {
	for i := 0; i < len(key); i++ {
		if b := key[i]; b < 0x20 || b == 0x7f {
			return fmt.Errorf("contains a control character (%#02x at byte %d)", b, i)
		}
	}
	return nil
}

// ResolveAPIKey returns the API key, preferring the environment.
//
// A key that exists but cannot be used is reported as ErrUnusableAPIKey
// rather than as a generic failure, because the two are recoverable in the
// same way — by collecting a new one — and only a distinguishable error lets
// the TUI offer that instead of dead-ending. Before this, a saved key with a
// NUL in it refused every launch AND never re-opened the key prompt, since
// that prompt is offered only when no key is configured at all: the tool
// could not fix, from inside itself, a file it had written.
func ResolveAPIKey(cfg *Config) (string, error) {
	if key := os.Getenv(APIKeyEnvVar); key != "" {
		if err := ValidateAPIKey(key); err != nil {
			return "", fmt.Errorf("%w: the key in %s %v; unset or correct it",
				ErrUnusableAPIKey, APIKeyEnvVar, err)
		}
		return key, nil
	}
	if cfg != nil && cfg.APIKey != "" {
		if err := ValidateAPIKey(cfg.APIKey); err != nil {
			// Naming the file is the point. encoding/json escapes a control
			// character on save and decodes it back on load, so the value
			// looks correct in any editor — a user told only "your key is
			// wrong" has nowhere to go.
			where := "your config file"
			if path, perr := Path(); perr == nil {
				where = path
			}
			return "", fmt.Errorf("%w: the saved key %v. Re-enter it, or remove \"api_key\" from %s",
				ErrUnusableAPIKey, err, where)
		}
		return cfg.APIKey, nil
	}
	return "", fmt.Errorf("%w: set %s or run with a saved key (get one at https://openrouter.ai/keys)",
		ErrNoAPIKey, APIKeyEnvVar)
}

// APIKey resolves the credential a launch carries, from the environment or
// the saved settings. It is the adapter internal/launch is wired with: the
// planner takes a func() (string, error) so it names no settings store of
// its own, and this is what that func is for this tool.
//
// The error identity matters and is deliberately not wrapped: ErrNoAPIKey
// reaches the TUI through launch.Plan and is what opens the key prompt in
// place rather than aborting the session.
func APIKey() (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	return ResolveAPIKey(cfg)
}

// RecordSelection persists the agent and model just launched, for the next
// run to preselect.
//
// The config is re-read here rather than threaded through from the plan: in
// a TUI a profile may have been added between planning and launching, and
// that edit must not be clobbered.
func RecordSelection(agentName, modelID string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.LastAgent = agentName
	cfg.LastModel = modelID
	return Save(cfg)
}
