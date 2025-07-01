package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var metaMutex = &sync.Mutex{}

func getMetaDir(baseDir string) string {
	return filepath.Join(GetBtoolDir(baseDir), "meta")
}

func getCounterPath(baseDir string) string {
	return filepath.Join(getMetaDir(baseDir), "counter")
}

// getNextSnapID is the internal, non-locking implementation of GetNextSnapID.
// It should only be called by functions that already hold the metaMutex.
func getNextSnapID(baseDir string) (int64, error) {
	counterPath := getCounterPath(baseDir)
	content, err := os.ReadFile(counterPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If the counter doesn't exist, the first ID is 1.
			return 1, nil
		}
		return 0, err
	}

	trimmedContent := strings.TrimSpace(string(content))
	if trimmedContent == "" {
		// If the file is empty, the next ID is 1.
		return 1, nil
	}

	id, err := strconv.ParseInt(trimmedContent, 10, 64)
	if err != nil {
		// If the file is corrupt (not a valid int), we can't proceed.
		return 0, fmt.Errorf("corrupt counter file: %w", err)
	}
	return id, nil
}

// GetNextSnapID is the public, thread-safe function to read the next snapshot ID.
func GetNextSnapID(baseDir string) (int64, error) {
	metaMutex.Lock()
	defer metaMutex.Unlock()
	return getNextSnapID(baseDir)
}

// IncrementNextSnapID increments the persistent counter for the next snapshot ID.
// This function is thread-safe.
func IncrementNextSnapID(baseDir string) error {
	metaMutex.Lock()
	defer metaMutex.Unlock()

	// Ensure the directory exists before we try to write to it.
	metaDir := getMetaDir(baseDir)
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return err
	}

	// Call the internal, non-locking function since we already have the lock.
	currentID, err := getNextSnapID(baseDir)
	if err != nil {
		return err
	}

	nextID := currentID + 1
	counterPath := getCounterPath(baseDir)
	return os.WriteFile(counterPath, []byte(strconv.FormatInt(nextID, 10)), 0644)
}
