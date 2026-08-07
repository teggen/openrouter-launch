// Command openrouter-launch starts coding agents against OpenRouter models.
package main

import (
	"os"

	"github.com/teggen/openrouter-launch/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
