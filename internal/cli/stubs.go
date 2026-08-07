package cli

import "github.com/spf13/cobra"

// Temporary stub, replaced in Task 13.
func newProfileCmd(*globalFlags) *cobra.Command {
	return &cobra.Command{Use: "profile", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
