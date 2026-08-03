// Package cmd
/*
Copyright © 2026 Kareylo <contact@kareylo.fr>
*/
package cmd

import (
	"dackup/cmd/backup"
	"dackup/cmd/config"
	"dackup/cmd/restore"
	"dackup/internal/shared"
	"os"

	"github.com/spf13/cobra"
)

var options = &shared.Options{}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dackup",
	Short: "Small application to backup your docker containers data through rsync",
	Long: `Small CLI application to backup your docker containers data through rsync.
You need docker and rsync installed on your system.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) {},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&options.Verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&options.DryRun, "dry-run", "d", false, "preview actions without writing files")

	rootCmd.AddCommand(backup.NewCommand(options))
	rootCmd.AddCommand(config.NewCommand(options))
	rootCmd.AddCommand(restore.NewCommand(options))
}
