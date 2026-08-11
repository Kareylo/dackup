// Package version exposes dackup's version string and its "version" command.
package version

import (
	"dackup/internal/shared"
	"fmt"

	"github.com/spf13/cobra"
)

// Version is dackup's version string. It defaults to "dev" for local builds
// and is overridden at build time via -ldflags "-X dackup/cmd/version.Version=...".
var Version = "dev"

// NewCommand builds the "version" command, which prints Version.
func NewCommand(options *shared.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the dackup version",
		Long:  "Print the dackup version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "dackup version %s\n", Version)
			return nil
		},
	}
}
