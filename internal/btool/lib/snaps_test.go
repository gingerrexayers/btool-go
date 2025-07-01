package lib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// setupSnapsTest creates a temporary test directory with a .btool/snaps subdirectory.
// It also provides a helper function to easily create snap files for testing.
func setupSnapsTest(t *testing.T) (string, func(id int64, hash, timestamp, message string)) {
	testDir := t.TempDir()
	snapsDir := GetSnapsDir(testDir)
	err := os.MkdirAll(snapsDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create snaps test directory: %v", err)
	}

	// createSnapFile is a helper to reduce boilerplate in tests.
	createSnapFile := func(id int64, hash, timestamp, message string) {
		snapData := types.Snap{
			ID:           id,
			Timestamp:    timestamp,
			Message:      message,
			RootTreeHash: "dummyTreeHash",
			SourceSize:   1024,
		}
		content, err := json.Marshal(snapData)
		if err != nil {
			t.Fatalf("Failed to marshal snap data: %v", err)
		}
		err = os.WriteFile(filepath.Join(snapsDir, hash+".json"), content, 0644)
		if err != nil {
			t.Fatalf("Failed to write snap file %s.json: %v", hash, err)
		}
	}

	return testDir, createSnapFile
}

func TestGetSortedSnaps(t *testing.T) {
	t.Run("should correctly sort snaps by ID", func(t *testing.T) {
		// Arrange
		testDir, createSnapFile := setupSnapsTest(t)
		// Create snaps with IDs out of order to test sorting.
		createSnapFile(2, "hash_2", "2023-01-02T12:00:00Z", "second snap")
		createSnapFile(3, "hash_3", "2023-01-03T12:00:00Z", "third snap")
		createSnapFile(1, "hash_1", "2023-01-01T12:00:00Z", "first snap")

		// Act
		snaps, err := GetSortedSnaps(testDir)
		if err != nil {
			t.Fatalf("GetSortedSnaps failed unexpectedly: %v", err)
		}

		// Assert
		if len(snaps) != 3 {
			t.Fatalf("Expected 3 snaps, but got %d", len(snaps))
		}
		// Check that snaps are sorted by ID (1, 2, 3).
		if snaps[0].ID != 1 || snaps[0].Hash != "hash_1" {
			t.Errorf("Expected first snap to have ID 1 and hash 'hash_1', got ID %d and hash '%s'", snaps[0].ID, snaps[0].Hash)
		}
		if snaps[1].ID != 2 || snaps[1].Hash != "hash_2" {
			t.Errorf("Expected second snap to have ID 2 and hash 'hash_2', got ID %d and hash '%s'", snaps[1].ID, snaps[1].Hash)
		}
		if snaps[2].ID != 3 || snaps[2].Hash != "hash_3" {
			t.Errorf("Expected third snap to have ID 3 and hash 'hash_3', got ID %d and hash '%s'", snaps[2].ID, snaps[2].Hash)
		}
	})

	t.Run("should return empty slice when snaps directory does not exist", func(t *testing.T) {
		// Arrange
		testDir := t.TempDir() // A clean directory with no .btool/snaps subdir

		// Act
		snaps, err := GetSortedSnaps(testDir)

		// Assert
		if err != nil {
			t.Fatalf("Expected no error for missing snaps dir, but got: %v", err)
		}
		if len(snaps) != 0 {
			t.Errorf("Expected 0 snaps for missing directory, but got %d", len(snaps))
		}
	})

	t.Run("should skip corrupted and invalid files gracefully", func(t *testing.T) {
		// Arrange
		testDir, createSnapFile := setupSnapsTest(t)
		snapsDir := GetSnapsDir(testDir)

		// Create one valid snap.
		createSnapFile(1, "hash_valid", "2023-01-01T12:00:00Z", "valid snap")

		// Create a file with invalid JSON.
		err := os.WriteFile(filepath.Join(snapsDir, "corrupted.json"), []byte("{ not valid json }"), 0644)
		if err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		// Create a file with a bad timestamp.
		createSnapFile(2, "hash_bad_ts", "not-a-valid-timestamp", "bad timestamp")

		// Create a non-JSON file that should be ignored.
		err = os.WriteFile(filepath.Join(snapsDir, "ignore_me.txt"), []byte("text file"), 0644)
		if err != nil {
			t.Fatalf("Failed to write text file: %v", err)
		}

		// Act
		snaps, err := GetSortedSnaps(testDir)

		// Assert
		if err != nil {
			t.Fatalf("GetSortedSnaps failed unexpectedly: %v", err)
		}
		// It should have skipped the two bad .json files and the .txt file, leaving only the valid one.
		if len(snaps) != 1 {
			t.Fatalf("Expected 1 valid snap, but found %d", len(snaps))
		}
		if snaps[0].Hash != "hash_valid" {
			t.Errorf("Expected the only found snap to be 'hash_valid', got '%s'", snaps[0].Hash)
		}
		if snaps[0].ID != 1 {
			t.Errorf("Expected the valid snap to have ID 1, got %d", snaps[0].ID)
		}
	})

	t.Run("should correctly parse all fields from a valid snap file", func(t *testing.T) {
		// Arrange
		testDir, createSnapFile := setupSnapsTest(t)
		timestamp := "2025-06-28T10:00:00Z"
		createSnapFile(1, "abcdef123", timestamp, "test message")

		expectedTime, _ := time.Parse(time.RFC3339, timestamp)

		// Act
		snaps, err := GetSortedSnaps(testDir)
		if err != nil {
			t.Fatalf("GetSortedSnaps failed: %v", err)
		}
		if len(snaps) != 1 {
			t.Fatalf("Expected 1 snap, got %d", len(snaps))
		}

		// Assert
		result := snaps[0]
		if result.ID != 1 {
			t.Errorf("ID mismatch: got %d, want 1", result.ID)
		}
		if result.Hash != "abcdef123" {
			t.Errorf("Hash mismatch: got %s, want abcdef123", result.Hash)
		}
		if !result.Timestamp.Equal(expectedTime) {
			t.Errorf("Timestamp mismatch: got %v, want %v", result.Timestamp, expectedTime)
		}
		if result.Message != "test message" {
			t.Errorf("Message mismatch: got %s, want 'test message'", result.Message)
		}
		if result.RootTreeHash != "dummyTreeHash" {
			t.Errorf("RootTreeHash mismatch: got %s, want 'dummyTreeHash'", result.RootTreeHash)
		}
		if result.SourceSize != 1024 {
			t.Errorf("SourceSize mismatch: got %d, want 1024", result.SourceSize)
		}
	})
}
