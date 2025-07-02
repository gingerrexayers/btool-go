package lib

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSnapsTest creates a temporary test directory with a .btool/snaps subdirectory.
// It also provides a helper function to easily create snap files for testing.
func setupSnapsTest(t *testing.T) (string, func(id int64, hash, timestamp, message string)) {
	t.Helper()
	testDir := t.TempDir()
	snapsDir := GetSnapsDir(testDir)
	err := os.MkdirAll(snapsDir, 0755)
	require.NoError(t, err, "Failed to create snaps test directory")

	// createSnapFile is a helper to reduce boilerplate in tests.
	createSnapFile := func(id int64, hash, timestamp, message string) {
		t.Helper()
		snapData := types.Snap{
			ID:           id,
			Timestamp:    timestamp,
			Message:      message,
			RootTreeHash: "dummyTreeHash",
			SourceSize:   1024,
		}
		content, err := json.Marshal(snapData)
		require.NoError(t, err, "Failed to marshal snap data")

		err = os.WriteFile(filepath.Join(snapsDir, hash+".json"), content, 0644)
		require.NoError(t, err, "Failed to write snap file %s.json", hash)
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
		require.NoError(t, err, "GetSortedSnaps failed unexpectedly")

		// Assert
		require.Len(t, snaps, 3, "Expected 3 snaps")
		// Check that snaps are sorted by ID (1, 2, 3).
		assert.Equal(t, int64(1), snaps[0].ID, "Expected first snap to have ID 1")
		assert.Equal(t, "hash_1", snaps[0].Hash, "Expected first snap to have hash 'hash_1'")
		assert.Equal(t, int64(2), snaps[1].ID, "Expected second snap to have ID 2")
		assert.Equal(t, "hash_2", snaps[1].Hash, "Expected second snap to have hash 'hash_2'")
		assert.Equal(t, int64(3), snaps[2].ID, "Expected third snap to have ID 3")
		assert.Equal(t, "hash_3", snaps[2].Hash, "Expected third snap to have hash 'hash_3'")
	})

	t.Run("should return empty slice when snaps directory does not exist", func(t *testing.T) {
		// Arrange
		testDir := t.TempDir() // A clean directory with no .btool/snaps subdir

		// Act
		snaps, err := GetSortedSnaps(testDir)

		// Assert
		require.NoError(t, err, "Expected no error for missing snaps dir")
		assert.Empty(t, snaps, "Expected 0 snaps for missing directory")
	})

	t.Run("should skip corrupted and invalid files gracefully", func(t *testing.T) {
		// Arrange
		testDir, createSnapFile := setupSnapsTest(t)
		snapsDir := GetSnapsDir(testDir)

		// Create one valid snap.
		createSnapFile(1, "hash_valid", "2023-01-01T12:00:00Z", "valid snap")

		// Create a file with invalid JSON.
		err := os.WriteFile(filepath.Join(snapsDir, "corrupted.json"), []byte("{ not valid json }"), 0644)
		require.NoError(t, err, "Failed to write corrupted file")

		// Create a file with a bad timestamp.
		createSnapFile(2, "hash_bad_ts", "not-a-valid-timestamp", "bad timestamp")

		// Create a non-JSON file that should be ignored.
		err = os.WriteFile(filepath.Join(snapsDir, "ignore_me.txt"), []byte("text file"), 0644)
		require.NoError(t, err, "Failed to write text file")

		// Act
		snaps, err := GetSortedSnaps(testDir)

		// Assert
		require.NoError(t, err, "GetSortedSnaps failed unexpectedly")
		// It should have skipped the two bad .json files and the .txt file, leaving only the valid one.
		require.Len(t, snaps, 1, "Expected 1 valid snap")
		assert.Equal(t, "hash_valid", snaps[0].Hash, "Expected the only found snap to be 'hash_valid'")
		assert.Equal(t, int64(1), snaps[0].ID, "Expected the valid snap to have ID 1")
	})

	t.Run("should correctly parse all fields from a valid snap file", func(t *testing.T) {
		// Arrange
		testDir, createSnapFile := setupSnapsTest(t)
		timestamp := "2025-06-28T10:00:00Z"
		createSnapFile(1, "abcdef123", timestamp, "test message")

		expectedTime, err := time.Parse(time.RFC3339, timestamp)
		require.NoError(t, err)

		// Act
		snaps, err := GetSortedSnaps(testDir)
		require.NoError(t, err, "GetSortedSnaps failed")
		require.Len(t, snaps, 1, "Expected 1 snap")

		// Assert
		result := snaps[0]
		assert.Equal(t, int64(1), result.ID, "ID mismatch")
		assert.Equal(t, "abcdef123", result.Hash, "Hash mismatch")
		assert.True(t, result.Timestamp.Equal(expectedTime), "Timestamp mismatch")
		assert.Equal(t, "test message", result.Message, "Message mismatch")
		assert.Equal(t, "dummyTreeHash", result.RootTreeHash, "RootTreeHash mismatch")
		assert.Equal(t, int64(1024), result.SourceSize, "SourceSize mismatch")
	})
}
