package main

import (
	"fmt"
	"os"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/spf13/cobra"
)

// snapshotCompletions provides dynamic tab completion for snapshot identifiers.
// It suggests both numeric IDs and unique hash prefixes.
func snapshotCompletions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// This completion function is for the first argument only.
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Determine the repository directory.
	dir, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	// The directory flag can override the current working directory.
	if dirFlag, err := cmd.Flags().GetString("directory"); err == nil && dirFlag != "" {
		dir = dirFlag
	}

	// Get the list of sorted snapshots.
	snaps, err := lib.GetSortedSnaps(dir)
	if err != nil {
		// Don't return an error, just fail to complete.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Create a list of suggestions.
	var suggestions []string
	for _, snap := range snaps {
		timestamp := snap.Timestamp.Format("2006-01-02 15:04:05")
		suggestions = append(suggestions, fmt.Sprintf("%d\t%s %s - %s", snap.ID, snap.Hash[:7], timestamp, snap.Message))
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp
}
