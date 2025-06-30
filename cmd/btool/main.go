package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{Use: "btool"}

	// Add commands
	rootCmd.AddCommand(NewSnapCommand())
	rootCmd.AddCommand(NewListCommand())
	rootCmd.AddCommand(NewRestoreCommand())
	rootCmd.AddCommand(NewPruneCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
