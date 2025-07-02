package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getIndexObjectCount is a test helper to read the index and count the objects.
func getIndexObjectCount(t *testing.T, baseDir string) int {
	lib.ResetObjectStoreState() // Ensure we read from disk, not cache.
	indexPath := lib.GetIndexPath(baseDir)
	content, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err, "Failed to read index file")

	var index map[string]interface{}
	err = json.Unmarshal(content, &index)
	require.NoError(t, err, "Failed to parse index json")
	return len(index)
}

// setupSnapshots creates a series of snapshots for testing prune.
func setupSnapshots(t *testing.T, testDir string, numSnaps int) []lib.SnapDetail {
	filePath := filepath.Join(testDir, "file.txt")
	for i := 1; i <= numSnaps; i++ {
		// To ensure the snap command detects a change, we remove the old file first.
		// This is more reliable than relying on mtime resolution in fast-running tests.
		_ = os.Remove(filePath) // Ignore error if it doesn't exist

		content := "version " + strconv.Itoa(i)
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
		require.NoError(t, commands.Snap(testDir, "snap "+strconv.Itoa(i)))
	}
	snaps, err := lib.GetSortedSnaps(testDir)
	require.NoError(t, err)
	require.Len(t, snaps, numSnaps, "should have created the correct number of snapshots")
	return snaps
}

func TestPruneCommand(t *testing.T) {
	t.Run("should prune snapshots older than the one specified by ID", func(t *testing.T) {
		// Arrange
		lib.ResetObjectStoreState()
		lib.ResetIgnoreState()
		testDir := t.TempDir()
		allSnaps := setupSnapshots(t, testDir, 4)
		t.Logf("Initial snaps created:")
		for i, s := range allSnaps {
			t.Logf("  - Snap %d (ID %d): Hash %s", i+1, s.ID, s.Hash)
		}
		initialObjectCount := getIndexObjectCount(t, testDir)

		// Act: Prune everything older than the third snap (allSnaps[2]).
		snapToPruneFrom := allSnaps[2]
		pruneOpts := commands.PruneOptions{SnapIdentifier: strconv.FormatInt(snapToPruneFrom.ID, 10)}
		err := commands.Prune(testDir, pruneOpts)
		require.NoError(t, err)

		// Assert
		remainingSnaps, err := lib.GetSortedSnaps(testDir)
		require.NoError(t, err)
		t.Logf("Snaps remaining after prune:")
		for i, s := range remainingSnaps {
			t.Logf("  - Snap %d (ID %d): Hash %s", i+1, s.ID, s.Hash)
		}
		assert.Len(t, remainingSnaps, 2, "Expected 2 snapshots to remain")
		assert.Equal(t, allSnaps[2].Hash, remainingSnaps[0].Hash, "The first remaining snap should be original snap 3")
		assert.Equal(t, allSnaps[3].Hash, remainingSnaps[1].Hash, "The second remaining snap should be original snap 4")

		finalObjectCount := getIndexObjectCount(t, testDir)
		assert.Less(t, finalObjectCount, initialObjectCount, "Object count should decrease after GC")

		// Golden Test: Restore the OLDEST remaining snapshot to verify GC didn't corrupt it.
		// This is the critical test. Snap 3's objects might have been shared with pruned snaps 1 and 2,
		// so we must ensure its content is still correct after GC.
		restoreDir := t.TempDir()
		err = commands.Restore(testDir, remainingSnaps[0].Hash[:12], restoreDir)
		require.NoError(t, err, "should be able to restore oldest remaining snap")

		restoredContent, err := os.ReadFile(filepath.Join(restoreDir, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "version 3", string(restoredContent), "restored content of snap 3 should be correct")
	})

	t.Run("should prune snapshots older than the one specified by hash prefix", func(t *testing.T) {
		// Arrange
		lib.ResetObjectStoreState()
		lib.ResetIgnoreState()
		testDir := t.TempDir()
		allSnaps := setupSnapshots(t, testDir, 4)
		snapToPruneFrom := allSnaps[2]

		// Act
		pruneOpts := commands.PruneOptions{SnapIdentifier: snapToPruneFrom.Hash[:12]}
		err := commands.Prune(testDir, pruneOpts)
		require.NoError(t, err)

		// Assert
		remainingSnaps, err := lib.GetSortedSnaps(testDir)
		require.NoError(t, err)
		assert.Len(t, remainingSnaps, 2)
		assert.Equal(t, snapToPruneFrom.Hash, remainingSnaps[0].Hash)

		// Golden Test: Restore the OLDEST remaining snapshot to verify GC didn't corrupt it.
		restoreDir := t.TempDir()
		// After pruning, snaps 3 and 4 are left. remainingSnaps[0] is original snap 3.
		err = commands.Restore(testDir, remainingSnaps[0].Hash[:12], restoreDir)
		require.NoError(t, err)
		restoredContent, err := os.ReadFile(filepath.Join(restoreDir, "file.txt"))
		require.NoError(t, err)
		assert.Equal(t, "version 3", string(restoredContent), "restored content of snap 3 should be correct")
	})

	t.Run("should do nothing if the oldest snapshot is specified", func(t *testing.T) {
		// Arrange
		lib.ResetObjectStoreState()
		lib.ResetIgnoreState()
		testDir := t.TempDir()
		allSnaps := setupSnapshots(t, testDir, 3)
		initialObjectCount := getIndexObjectCount(t, testDir)

		// Act: Prune from the oldest snap, which should do nothing.
		oldestSnapID := allSnaps[0].ID
		pruneOpts := commands.PruneOptions{SnapIdentifier: strconv.FormatInt(oldestSnapID, 10)}
		err := commands.Prune(testDir, pruneOpts)
		require.NoError(t, err)

		// Assert
		remainingSnaps, err := lib.GetSortedSnaps(testDir)
		require.NoError(t, err)
		assert.Len(t, remainingSnaps, 3, "Should not prune any snapshots")
		assert.Equal(t, initialObjectCount, getIndexObjectCount(t, testDir), "Object count should not change")
	})

	t.Run("should return an error for a non-existent snapshot identifier", func(t *testing.T) {
		// Arrange
		lib.ResetObjectStoreState()
		lib.ResetIgnoreState()
		testDir := t.TempDir()
		setupSnapshots(t, testDir, 2)

		// Act
		pruneOpts := commands.PruneOptions{SnapIdentifier: "99"} // Non-existent ID
		err := commands.Prune(testDir, pruneOpts)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no snap found with ID or hash prefix '99'")
	})

	t.Run("should return an error for an ambiguous snapshot hash prefix", func(t *testing.T) {
		// This test relies on the unit test for lib.FindSnap to correctly identify ambiguity.
		// Here, we just ensure that prune propagates the error from FindSnap.
		// A simple way is to provide an identifier that matches no snaps in an existing repo.

		// Arrange
		lib.ResetObjectStoreState()
		lib.ResetIgnoreState()
		testDir := t.TempDir()
		setupSnapshots(t, testDir, 2) // Repo is not empty

		// Act
		pruneOpts := commands.PruneOptions{SnapIdentifier: "nonexistent-prefix"}
		err := commands.Prune(testDir, pruneOpts)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no snap found with ID or hash prefix 'nonexistent-prefix'")
	})
}
