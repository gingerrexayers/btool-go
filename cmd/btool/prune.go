package main

import (
	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/spf13/cobra"
)

// NewPruneCommand creates the 'prune' command for the CLI.
func NewPruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune <snap-identifier> [directory]",
		Short: "Remove snapshots older than the specified one.",
		Long: `Prunes the backup repository by removing all snapshots older than the
specified snapshot and safely garbage-collecting all data that is no longer
referenced by any of the kept snapshots.`,
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: snapshotCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The first argument is the snapshot identifier.
			snapIdentifier := args[0]

			// The second, optional argument is the directory.
			dir := "."
			if len(args) > 1 {
				dir = args[1]
			}

			opts := commands.PruneOptions{SnapIdentifier: snapIdentifier}
			return commands.Prune(dir, opts)
		},
	}

	return cmd
}
