package cli

import "github.com/spf13/cobra"

// Temporary stubs, replaced in Tasks 12-13.
func newProfileCmd(*globalFlags) *cobra.Command {
	return &cobra.Command{Use: "profile", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}

func newLaunchCmds(*globalFlags) []*cobra.Command { return nil }
