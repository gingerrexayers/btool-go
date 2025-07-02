package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRestoreTest creates a test repository with a single snapshot.
// It returns the path to the source repo.
func setupRestoreTest(t *testing.T) (sourceDir string) {
	t.Helper()
	sourceDir = t.TempDir()

	// Create a file structure to be backed up.
	// Give a file non-default permissions to test mode restoration.
	// On Windows, file permissions behave differently, so this test is more meaningful on POSIX systems.
	var fileMode os.FileMode = 0744
	if runtime.GOOS == "windows" {
		fileMode = 0666
	}

	err := os.WriteFile(filepath.Join(sourceDir, "fileA.txt"), []byte("restore me"), fileMode)
	require.NoError(t, err, "Failed to write test file")

	err = os.Mkdir(filepath.Join(sourceDir, "subdir"), 0755)
	require.NoError(t, err, "Failed to create subdir")

	err = os.WriteFile(filepath.Join(sourceDir, "subdir", "fileB.txt"), []byte("me too"), 0644)
	require.NoError(t, err, "Failed to write nested test file")

	// Create the snapshot.
	err = commands.Snap(sourceDir, "restore test snap")
	require.NoError(t, err, "Setup failed: snap command failed")

	return sourceDir
}

// compareDirs checks if two directories have identical content, structure, and permissions.
func compareDirs(t *testing.T, dir1, dir2 string) {
	t.Helper()
	err := filepath.WalkDir(dir1, func(path1 string, d os.DirEntry, err error) error {
		require.NoError(t, err)

		// Get the corresponding path in the second directory.
		relPath, err := filepath.Rel(dir1, path1)
		require.NoError(t, err)

		// The .btool directory is part of the source repo but is not restored.
		// We must skip it during comparison.
		if relPath == lib.BtoolDirName {
			return filepath.SkipDir
		}

		path2 := filepath.Join(dir2, relPath)

		info1, err := d.Info()
		require.NoError(t, err)

		info2, err := os.Stat(path2)
		require.NoError(t, err, "Path '%s' exists in original but not in restored dir", relPath)

		assert.Equal(t, info1.IsDir(), info2.IsDir(), "Path '%s' is a directory in one tree but a file in the other", relPath)

		// Skip mode check on directories on Windows as it's not reliable.
		if !info1.IsDir() || runtime.GOOS != "windows" {
			assert.Equal(t, info1.Mode(), info2.Mode(), "Permission mismatch for '%s': original %v, restored %v", relPath, info1.Mode(), info2.Mode())
		}

		if !info1.IsDir() {
			content1, err1 := os.ReadFile(path1)
			content2, err2 := os.ReadFile(path2)
			require.NoError(t, err1, "Could not read file contents for '%s'", relPath)
			require.NoError(t, err2, "Could not read file contents for '%s'", relPath)
			assert.Equal(t, string(content1), string(content2), "Content mismatch for file '%s'", relPath)
		}

		return nil
	})
	require.NoError(t, err, "Failed to walk directory for comparison")
}

func TestRestoreCommand(t *testing.T) {
	t.Parallel()
	t.Run("should correctly restore a snapshot to an existing directory", func(t *testing.T) {
		lib.ResetObjectStoreState()
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputDir := t.TempDir() // Use a separate temp dir for output.
		snapID := 1              // The first snapshot created always has ID 1.

		// Act
		err := commands.Restore(sourceDir, strconv.Itoa(snapID), outputDir)
		require.NoError(t, err, "commands.Restore() returned an unexpected error")

		// Assert: The contents of the original sourceDir and the outputDir should be identical.
		compareDirs(t, sourceDir, outputDir)
	})

	t.Run("should create the output directory if it does not exist", func(t *testing.T) {
		lib.ResetObjectStoreState()
		// Arrange
		sourceDir := setupRestoreTest(t)
		nonExistentOutputDir := filepath.Join(t.TempDir(), "new_output")
		snapID := 1

		// Act
		err := commands.Restore(sourceDir, strconv.Itoa(snapID), nonExistentOutputDir)
		require.NoError(t, err, "commands.Restore() returned an unexpected error")

		// Assert
		assert.DirExists(t, nonExistentOutputDir, "Output directory was not created")
		compareDirs(t, sourceDir, nonExistentOutputDir)
	})

	t.Run("should fail if the output directory is a file", func(t *testing.T) {
		lib.ResetObjectStoreState()
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputFile := filepath.Join(t.TempDir(), "output_is_a_file")
		err := os.WriteFile(outputFile, []byte("I am a file"), 0644)
		require.NoError(t, err, "Failed to create output file for testing")
		snapID := 1

		// Act
		err = commands.Restore(sourceDir, strconv.Itoa(snapID), outputFile)

		// Assert
		require.Error(t, err, "Expected an error when output path is a file, but got nil")
		assert.Contains(t, err.Error(), "output path exists and is not a directory")
	})

	t.Run("should return an error for a non-existent snapshot ID", func(t *testing.T) {
		lib.ResetObjectStoreState()
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputDir := t.TempDir()
		nonExistentSnapID := "999"

		// Act
		err := commands.Restore(sourceDir, nonExistentSnapID, outputDir)

		// Assert
		require.Error(t, err, "Expected an error for a non-existent snapshot, but got nil")
		assert.Contains(t, err.Error(), "no snap found")
	})

	t.Run("should delete extraneous files in the destination directory", func(t *testing.T) {
		// Arrange
		sourceDir := t.TempDir()
		// Create a file and take a snapshot
		fileToKeepPath := filepath.Join(sourceDir, "file_to_keep.txt")
		err := os.WriteFile(fileToKeepPath, []byte("i should exist"), 0644)
		require.NoError(t, err, "failed to write file to keep")

		err = commands.Snap(sourceDir, "snap with one file")
		require.NoError(t, err, "snap failed")

		// Prepare the restore destination with an extra file
		restoreDir := t.TempDir()
		fileToDeletePath := filepath.Join(restoreDir, "file_to_delete.txt")
		err = os.WriteFile(fileToDeletePath, []byte("i should be deleted"), 0644)
		require.NoError(t, err, "failed to write file to delete")

		// Act
		err = commands.Restore(sourceDir, "1", restoreDir)
		require.NoError(t, err, "Restore command failed")

		// Assert
		// The file from the snapshot should exist
		assert.FileExists(t, filepath.Join(restoreDir, "file_to_keep.txt"), "File that should have been restored does not exist")

		// The extraneous file should have been deleted
		assert.NoFileExists(t, fileToDeletePath, "Extraneous file was not deleted from the restore directory")
	})

	t.Run("should fail gracefully if an object is missing from the index", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t) // This creates a snap with a few objects
		outputDir := t.TempDir()

		// Find a chunk object hash to remove from the index.
		store := lib.NewObjectStore(sourceDir)
		snaps, err := lib.GetSortedSnaps(sourceDir)
		require.NoError(t, err)
		require.NotEmpty(t, snaps, "Failed to get snaps to find an object to delete")

		rootTreeHash := snaps[0].RootTreeHash

		var rootTree types.Tree
		err = store.ReadObjectAsJSON(rootTreeHash, &rootTree)
		require.NoError(t, err, "Could not read root tree to find a file manifest")

		var fileManifestHash string
		var found bool
		for _, entry := range rootTree.Entries {
			if entry.Name == "fileA.txt" {
				require.Equal(t, "blob", entry.Type, "Expected 'fileA.txt' to be a blob")
				fileManifestHash = entry.Hash
				found = true
				break
			}
		}
		require.True(t, found, "Could not find entry for 'fileA.txt' in the root tree")

		var fileManifest types.FileManifest
		err = store.ReadObjectAsJSON(fileManifestHash, &fileManifest)
		require.NoError(t, err, "Could not read file manifest to find a chunk to delete")
		require.NotEmpty(t, fileManifest.Chunks, "File manifest has no chunks to delete")

		objectToDelete := fileManifest.Chunks[0].Hash

		// Now, corrupt the index by removing this object.
		indexPath := lib.GetIndexPath(sourceDir)
		indexContent, err := os.ReadFile(indexPath)
		require.NoError(t, err, "Failed to read index file")

		var index types.PackIndex
		err = json.Unmarshal(indexContent, &index)
		require.NoError(t, err, "Failed to unmarshal index for corruption")

		delete(index, objectToDelete)

		corruptedIndexJSON, err := json.MarshalIndent(index, "", "  ")
		require.NoError(t, err, "Failed to marshal corrupted index")

		err = os.WriteFile(indexPath, corruptedIndexJSON, 0644)
		require.NoError(t, err, "Failed to write corrupted index")

		// Act
		// The Restore command will create its own ObjectStore, which will load the now-corrupted index.
		err = commands.Restore(sourceDir, "1", outputDir)

		// Assert
		require.Error(t, err, "Expected restore to fail due to missing object, but it succeeded")
		assert.Contains(t, err.Error(), "not found in index", "Expected error about missing object from index")
	})
}
