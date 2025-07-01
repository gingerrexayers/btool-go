// Package commands contains the command-line interface for the btool application.
package commands

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	
	"github.com/gingerrexayers/btool-go/internal/btool/lib"
)

// formatBytes is a utility to convert bytes into a human-readable string (KB, MB, GB).
func formatBytes(bytes int64, decimals int) string {
	if bytes == 0 {
		return "0 Bytes"
	}
	const k = 1024
	if decimals < 0 {
		decimals = 0
	}
	sizes := []string{"Bytes", "KB", "MB", "GB", "TB"}
	
	i := int(math.Floor(math.Log(float64(bytes)) / math.Log(k)))
	if i >= len(sizes) {
		i = len(sizes) - 1
	}
	
	return fmt.Sprintf("%.*f %s", decimals, float64(bytes)/math.Pow(k, float64(i)), sizes[i])
}

// getStoredObjectsSize calculates the total size of all packfiles on disk.
func getStoredObjectsSize(baseDir string) (int64, error) {
	packsDir := lib.GetPacksDir(baseDir)
	dirEntries, err := os.ReadDir(packsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No packs exist yet.
		}
		return 0, err
	}

	var totalSize int64
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue // Skip files we can't get info for.
			}
			totalSize += info.Size()
		}
	}
	return totalSize, nil
}

// List is the main function for the 'list' command.
func List(targetDirectory string) error {
	absTargetPath, err := filepath.Abs(targetDirectory)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path for %s: %w", targetDirectory, err)
	}
	if _, err := os.Stat(absTargetPath); os.IsNotExist(err) {
		return fmt.Errorf("target directory does not exist: %s", absTargetPath)
	}
	

	// 1. Get all sorted snapshots using our new library function.
	snaps, err := lib.GetSortedSnaps(absTargetPath)
	if err != nil {
		return fmt.Errorf("failed to get snapshots: %w", err)
	}

	if len(snaps) == 0 {
		fmt.Printf("No snaps found for \"%s\".\n", absTargetPath)
		return nil
	}
	
	// 2. Calculate total stored size.
	totalStoredSize, err := getStoredObjectsSize(absTargetPath)
	if err != nil {
		return fmt.Errorf("failed to calculate stored size: %w", err)
	}

	// 3. Print the formatted table.
	fmt.Printf("Snaps for \"%s\":\n", absTargetPath)
	// Headers
	fmt.Printf("%-10s %-10s %-28s %-15s %-15s %s\n", "SNAPSHOT", "HASH", "TIMESTAMP", "SOURCE SIZE", "SNAP SIZE", "MESSAGE")
	// Separator
	fmt.Printf("%-10s %-10s %-28s %-15s %-15s %s\n", "=======", "=======", "=======================", "=============", "=============", "=======")

	for _, snap := range snaps {
		fmt.Printf("%-10s %-10s %-28s %-15s %-15s %s\n",
			strconv.FormatInt(snap.ID, 10),
			snap.Hash[:7],
			snap.Timestamp.Format("2006-01-02 15:04:05 MST"),
			formatBytes(snap.SourceSize, 2),
			formatBytes(snap.SnapSize, 2),
			snap.Message,
		)
	}
	
	fmt.Printf("\nTotal stored size of all objects: %s\n", formatBytes(totalStoredSize, 2))
	
	return nil
}