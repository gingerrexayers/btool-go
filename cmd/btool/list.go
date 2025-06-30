package main

import (
	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/spf13/cobra"
)

func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [directory]",
		Short: "List all available snaps for a directory.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return commands.List(dir)
		},
	}
	return cmd
}
