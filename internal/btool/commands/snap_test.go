// The _test suffix creates a special "external" test package, allowing us to
// test the 'commands' package's public API as a true black box.
package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	// We must now explicitly import the packages we are testing or using.
	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDir is a helper function to create a temporary directory structure for testing.
// It returns the root path of the test directory. Using t.TempDir() ensures automatic cleanup.
func setupTestDir(t *testing.T) string {
	t.Helper()
	// Reset global ignore state before each test run to ensure isolation.
	lib.ResetIgnoreState()

	testDir := t.TempDir()

	// Create a nested structure
	require.NoError(t, os.Mkdir(filepath.Join(testDir, "subdir"), 0755), "Failed to create subdir")

	// Create test files: two unique, two identical for de-duplication testing.
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "fileA.txt"), []byte("unique content A"), 0644), "Failed to write fileA.txt")
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "fileB.txt"), []byte("identical content"), 0644), "Failed to write fileB.txt")
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "subdir", "fileC.txt"), []byte("identical content"), 0644), "Failed to write subdir/fileC.txt")

	// Create an ignored file and the .btoolignore file to test the ignore logic.
	ignoreContent := "# Ignore log files via glob\n*.log\n\n# Ignore a specific directory\nignored_dir/"
	require.NoError(t, os.WriteFile(filepath.Join(testDir, ".btoolignore"), []byte(ignoreContent), 0644), "Failed to write .btoolignore")
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "app.log"), []byte("this log should be ignored"), 0644), "Failed to write app.log")
	require.NoError(t, os.Mkdir(filepath.Join(testDir, "ignored_dir"), 0755), "Failed to create ignored_dir")
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "ignored_dir", "some_file.txt"), []byte("this should also be ignored"), 0644), "Failed to write to ignored_dir")

	return testDir
}

// TestSnapCommand is a comprehensive integration test for the public Snap() function.
func TestSnapCommand(t *testing.T) {
	// 1. Setup the test environment.
	testDir := setupTestDir(t)

	// 2. Act: Call the public Snap function from the 'commands' package.
	err := commands.Snap(testDir, "My first integration test snap")
	require.NoError(t, err, "commands.Snap() failed unexpectedly")

	// 3. Assert - Check the filesystem state after the command has run.
	snapsDir := lib.GetSnapsDir(testDir)
	packsDir := lib.GetPacksDir(testDir)

	// Check if essential directories and files were created.
	require.DirExists(t, snapsDir, ".btool/snaps directory was not created")
	require.DirExists(t, packsDir, ".btool/packs directory was not created")

	// There should be exactly one snapshot manifest.
	snapFiles, err := os.ReadDir(snapsDir)
	require.NoError(t, err, "Could not read snaps directory")
	require.Len(t, snapFiles, 1, "Expected 1 snapshot file")

	// There should be exactly one pack file containing all objects.
	packFiles, err := os.ReadDir(packsDir)
	require.NoError(t, err, "Could not read packs directory")
	require.Len(t, packFiles, 1, "Expected 1 pack file")

	// 4. Assert - Deeply inspect the contents of the created snapshot.
	snapFileName := snapFiles[0].Name()
	snapContent, err := os.ReadFile(filepath.Join(snapsDir, snapFileName))
	require.NoError(t, err, "Could not read snapshot file")

	var snapData types.Snap
	err = json.Unmarshal(snapContent, &snapData)
	require.NoError(t, err, "Could not parse snapshot JSON")

	// Verify snapshot metadata.
	assert.Equal(t, "My first integration test snap", snapData.Message)
	// Total size of "unique content A" (16) + "identical content" (17) * 2 = 50
	expectedSize := int64(len("unique content A") + len("identical content")*2)
	assert.Equal(t, expectedSize, snapData.SourceSize)

	// Read and verify the root tree from the object store.
	store := lib.NewObjectStore(testDir)
	var rootTree types.Tree
	err = store.ReadObjectAsJSON(snapData.RootTreeHash, &rootTree)
	require.NoError(t, err, "Could not read root tree object from store")

	// The root tree should contain 2 files and 1 directory. 'app.log' and 'ignored_dir' must NOT be present.
	require.Len(t, rootTree.Entries, 3, "Expected root tree to have 3 entries (fileA, fileB, subdir)")

	var fileA, fileB, subDirEntry types.TreeEntry
	var foundA, foundB, foundSubDir bool
	for _, entry := range rootTree.Entries {
		switch entry.Name {
		case "fileA.txt":
			fileA = entry
			foundA = true
		case "fileB.txt":
			fileB = entry
			foundB = true
		case "subdir":
			subDirEntry = entry
			foundSubDir = true
		}
	}
	require.True(t, foundA && foundB && foundSubDir, "Root tree is missing one or more expected file/dir entries")
	assert.Equal(t, "tree", subDirEntry.Type, "Expected 'subdir' entry to be of type 'tree'")

	// Read and verify the sub-tree for the 'subdir' directory.
	var subTree types.Tree
	err = store.ReadObjectAsJSON(subDirEntry.Hash, &subTree)
	require.NoError(t, err, "Could not read sub-tree object")
	require.Len(t, subTree.Entries, 1, "Sub-tree should have exactly one entry")
	assert.Equal(t, "fileC.txt", subTree.Entries[0].Name, "Sub-tree has incorrect entries")

	// 5. Assert - De-duplication of content.
	// The manifest hash for fileB ("identical content") must be the same as fileC ("identical content").
	fileC := subTree.Entries[0]
	assert.Equal(t, fileB.Hash, fileC.Hash, "De-duplication failed: hashes for identical content should match")
	// And they must be different from fileA ("unique content A").
	assert.NotEqual(t, fileA.Hash, fileB.Hash, "Hashes for different content should not match")
}

func TestSnapCommand_EmptyDir(t *testing.T) {
	// Arrange: Create an empty directory.
	lib.ResetIgnoreState()
	testDir := t.TempDir()

	// Act: Take a snapshot of the empty directory.
	err := commands.Snap(testDir, "empty dir snap")
	require.NoError(t, err, "Snap command failed for an empty directory")

	// Assert: A snapshot was created.
	snaps, err := lib.GetSortedSnaps(testDir)
	require.NoError(t, err, "Failed to get snaps after snapshotting empty dir")
	require.Len(t, snaps, 1, "Expected 1 snapshot for empty dir")
	assert.Equal(t, "empty dir snap", snaps[0].Message, "Incorrect snap message")

	// Assert: The root tree is empty.
	store := lib.NewObjectStore(testDir)
	var rootTree types.Tree
	err = store.ReadObjectAsJSON(snaps[0].RootTreeHash, &rootTree)
	require.NoError(t, err, "Failed to read root tree of empty snap")
	assert.Empty(t, rootTree.Entries, "Expected root tree to be empty")

	// Act: Restore the snapshot to a new directory.
	outputDir := t.TempDir()
	err = commands.Restore(testDir, snaps[0].Hash, outputDir)
	require.NoError(t, err, "Failed to restore empty snapshot")

	// Assert: The restored directory is empty.
	files, err := os.ReadDir(outputDir)
	require.NoError(t, err, "Could not read restored directory")
	assert.Empty(t, files, "Restored directory is not empty")
}
