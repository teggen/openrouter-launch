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

// Filters is the persisted model-picker filter state. Zero numeric values
// mean the filter is unset.
type Filters struct {
	ToolsOnly  bool    `json:"tools_only"`
	FreeOnly   bool    `json:"free_only"`
	MinContext int     `json:"min_context"`
	MaxPrice   float64 `json:"max_price"`
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

// ResolveAPIKey returns the API key, preferring the environment.
func ResolveAPIKey(cfg *Config) (string, error) {
	if key := os.Getenv(APIKeyEnvVar); key != "" {
		return key, nil
	}
	if cfg != nil && cfg.APIKey != "" {
		return cfg.APIKey, nil
	}
	return "", fmt.Errorf("%w: set %s or run with a saved key (get one at https://openrouter.ai/keys)",
		ErrNoAPIKey, APIKeyEnvVar)
}
