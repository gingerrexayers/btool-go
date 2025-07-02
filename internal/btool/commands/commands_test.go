package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper function to create a temporary directory with a file
func createTestRepo(t *testing.T, content string) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "btool-test-")
	require.NoError(t, err, "Failed to create temp dir")

	if content != "" {
		err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644)
		require.NoError(t, err, "Failed to write test file")
	}

	// Return the directory path and a cleanup function
	return tmpDir, func() { os.RemoveAll(tmpDir) }
}

func TestSnapWithMessage(t *testing.T) {
	tmpDir, cleanup := createTestRepo(t, "hello world")
	defer cleanup()

	// 1. Create a snap with a message
	message := "this is a test message"
	err := Snap(tmpDir, message)
	require.NoError(t, err, "Snap() failed")

	// 2. Get the snaps and check the message
	snaps, err := lib.GetSortedSnaps(tmpDir)
	require.NoError(t, err, "GetSortedSnaps() failed")

	require.Len(t, snaps, 1)
	assert.Equal(t, message, snaps[0].Message)
}

func TestListDoesNotResetState(t *testing.T) {
	tmpDir, cleanup := createTestRepo(t, "file1")
	defer cleanup()

	// 1. Create an initial snap
	err := Snap(tmpDir, "first snap")
	require.NoError(t, err, "Snap() failed")

	// 2. Get the initial index state
	store := lib.NewObjectStore(tmpDir)
	index1, err := store.GetIndex()
	require.NoError(t, err, "GetIndex() failed")
	require.NotEmpty(t, index1, "Index is empty after first snap, should not be")

	// 3. Run the List command
	err = List(tmpDir)
	require.NoError(t, err, "List() failed")

	// 4. Get the index state again and compare
	// Re-create the store to simulate a separate command run
	store2 := lib.NewObjectStore(tmpDir)
	index2, err := store2.GetIndex()
	require.NoError(t, err, "GetIndex() failed after List()")

	assert.Len(t, index2, len(index1), "List command appears to have reset the state")
}
