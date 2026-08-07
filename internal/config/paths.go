// Package config persists openrouter-launch settings and named profiles.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const appDir = "openrouter-launch"

// Dir returns the configuration directory, honoring XDG_CONFIG_HOME.
func Dir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve config dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, appDir), nil
}

// Path returns the configuration file location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}
