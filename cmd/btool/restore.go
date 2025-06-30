package main

import (
	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/spf13/cobra"
)

// NewRestoreCommand creates the 'restore' command for the CLI.
func NewRestoreCommand() *cobra.Command {
	var sourceDir string
	var outputDir string

	cmd := &cobra.Command{
		Use:   "restore <snap_id_or_hash>",
		Short: "Restore a directory state from a snapshot.",
		Long: `Restores a snapshot to a specified directory. The target directory
will be modified to match the state of the snapshot.`,
		Args: cobra.ExactArgs(1), // Requires exactly one argument: the snapshot identifier.
		RunE: func(cmd *cobra.Command, args []string) error {
			snapIdentifier := args[0]

			// If output directory is not specified, it defaults to the source directory.
			finalOutputDir := outputDir
			if finalOutputDir == "" {
				finalOutputDir = sourceDir
			}

			// Call the core logic from the internal/btool/commands package.
			return commands.Restore(sourceDir, snapIdentifier, finalOutputDir)
		},
	}

	// Define flags for the command.
	cmd.Flags().StringVarP(&sourceDir, "directory", "d", ".", "The directory containing the .btool database")
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "The directory to restore files to (defaults to source directory)")

	return cmd
}
