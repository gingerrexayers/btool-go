package commands_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"encoding/json"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// setupRestoreTest creates a test repository with a single snapshot.
// It returns the path to the source repo.
func setupRestoreTest(t *testing.T) (sourceDir string) {
	sourceDir = t.TempDir()

	// Create a file structure to be backed up.
	// Give a file non-default permissions to test mode restoration.
	// On Windows, file permissions behave differently, so this test is more meaningful on POSIX systems.
	var fileMode os.FileMode = 0744
	if runtime.GOOS == "windows" {
		fileMode = 0666
	}

	err := os.WriteFile(filepath.Join(sourceDir, "fileA.txt"), []byte("restore me"), fileMode)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	err = os.Mkdir(filepath.Join(sourceDir, "subdir"), 0755)
	if err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	err = os.WriteFile(filepath.Join(sourceDir, "subdir", "fileB.txt"), []byte("me too"), 0644)
	if err != nil {
		t.Fatalf("Failed to write nested test file: %v", err)
	}

	// Create the snapshot.
	err = commands.Snap(sourceDir, "restore test snap")
	if err != nil {
		t.Fatalf("Setup failed: snap command failed: %v", err)
	}

	return sourceDir
}

// compareDirs checks if two directories have identical content, structure, and permissions.
func compareDirs(t *testing.T, dir1, dir2 string) {
	err := filepath.WalkDir(dir1, func(path1 string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get the corresponding path in the second directory.
		relPath, err := filepath.Rel(dir1, path1)
		if err != nil {
			return err
		}

		// The .btool directory is part of the source repo but is not restored.
		// We must skip it during comparison.
		if relPath == lib.BtoolDirName {
			return filepath.SkipDir
		}

		path2 := filepath.Join(dir2, relPath)

		info1, err := d.Info()
		if err != nil {
			return err
		}

		info2, err := os.Stat(path2)
		if err != nil {
			t.Errorf("Path '%s' exists in original but not in restored dir", relPath)
			return nil // Continue walking
		}

		if info1.IsDir() != info2.IsDir() {
			t.Errorf("Path '%s' is a directory in one tree but a file in the other", relPath)
		}

		// Skip mode check on directories on Windows as it's not reliable.
		if info1.Mode() != info2.Mode() && (!info1.IsDir() || runtime.GOOS != "windows") {
			t.Errorf("Permission mismatch for '%s': original %v, restored %v", relPath, info1.Mode(), info2.Mode())
		}

		if !info1.IsDir() {
			content1, err1 := os.ReadFile(path1)
			content2, err2 := os.ReadFile(path2)
			if err1 != nil || err2 != nil {
				t.Errorf("Could not read file contents for '%s'", relPath)
			}
			if string(content1) != string(content2) {
				t.Errorf("Content mismatch for file '%s'", relPath)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk directory for comparison: %v", err)
	}
}

func TestRestoreCommand(t *testing.T) {
	t.Parallel()
	t.Run("should correctly restore a snapshot to an existing directory", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputDir := t.TempDir() // Use a separate temp dir for output.
		snapID := 1              // The first snapshot created always has ID 1.

		// Act
		err := commands.Restore(sourceDir, strconv.Itoa(snapID), outputDir)
		if err != nil {
			t.Fatalf("commands.Restore() returned an unexpected error: %v", err)
		}

		// Assert: The contents of the original sourceDir and the outputDir should be identical.
		compareDirs(t, sourceDir, outputDir)
	})

	t.Run("should create the output directory if it does not exist", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t)
		nonExistentOutputDir := filepath.Join(t.TempDir(), "new_output")
		snapID := 1

		// Act
		err := commands.Restore(sourceDir, strconv.Itoa(snapID), nonExistentOutputDir)
		if err != nil {
			t.Fatalf("commands.Restore() returned an unexpected error: %v", err)
		}

		// Assert
		if _, err := os.Stat(nonExistentOutputDir); os.IsNotExist(err) {
			t.Fatal("Restore command did not create the output directory")
		}
		compareDirs(t, sourceDir, nonExistentOutputDir)
	})

	t.Run("should restore using a unique hash prefix", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputDir := t.TempDir()

		snaps, err := lib.GetSortedSnaps(sourceDir)
		if err != nil || len(snaps) == 0 {
			t.Fatal("Failed to get snaps for hash prefix test")
		}
		snapHash := snaps[0].Hash
		prefix := snapHash[:8] // Use the first 8 characters as a unique prefix.

		// Act
		err = commands.Restore(sourceDir, prefix, outputDir)
		if err != nil {
			t.Fatalf("commands.Restore() with hash prefix failed: %v", err)
		}

		// Assert
		compareDirs(t, sourceDir, outputDir)
	})

	t.Run("should return an error for a non-existent snapshot ID", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t)
		outputDir := t.TempDir()
		nonExistentID := "999"

		// Act
		err := commands.Restore(sourceDir, nonExistentID, outputDir)

		// Assert
		if err == nil {
			t.Fatal("Expected an error for a non-existent snapshot, but got nil")
		}
		if !strings.Contains(err.Error(), "no snap found") {
			t.Errorf("Expected error to mention 'no snap found', but got: %v", err)
		}
	})

	t.Run("should return an error for an ambiguous hash prefix", func(t *testing.T) {
		// Arrange
		sourceDir := t.TempDir()
		outputDir := t.TempDir()

		// Create a file to ensure snapshots are actually created.
		err := os.WriteFile(filepath.Join(sourceDir, "file.txt"), []byte("data"), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Create two snapshots to make prefixes potentially ambiguous.
		err = commands.Snap(sourceDir, "snap one")
		if err != nil {
			t.Fatalf("Failed to create first snap: %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
		err = commands.Snap(sourceDir, "snap two")
		if err != nil {
			t.Fatalf("Failed to create second snap: %v", err)
		}

		// Act: Try to restore with an empty string, which will match all snaps.
		err = commands.Restore(sourceDir, "", outputDir)

		// Assert
		if err == nil {
			t.Fatal("Expected an error for an ambiguous prefix, but got nil")
		}
		if !strings.Contains(err.Error(), "ambiguous snap identifier") {
			t.Errorf("Expected error to mention 'ambiguous snap identifier', but got: %v", err)
		}
	})

	t.Run("should delete extraneous files in the destination directory", func(t *testing.T) {
		// Arrange
		sourceDir := t.TempDir()
		// Create a file and take a snapshot
		fileToKeepPath := filepath.Join(sourceDir, "file_to_keep.txt")
		if err := os.WriteFile(fileToKeepPath, []byte("i should exist"), 0644); err != nil {
			t.Fatalf("failed to write file to keep: %v", err)
		}
		if err := commands.Snap(sourceDir, "snap with one file"); err != nil {
			t.Fatalf("snap failed: %v", err)
		}

		// Prepare the restore destination with an extra file
		restoreDir := t.TempDir()
		fileToDeletePath := filepath.Join(restoreDir, "file_to_delete.txt")
		if err := os.WriteFile(fileToDeletePath, []byte("i should be deleted"), 0644); err != nil {
			t.Fatalf("failed to write file to delete: %v", err)
		}

		// Act
		err := commands.Restore(sourceDir, "1", restoreDir)
		if err != nil {
			t.Fatalf("Restore command failed: %v", err)
		}

		// Assert
		// The file from the snapshot should exist
		if _, err := os.Stat(filepath.Join(restoreDir, "file_to_keep.txt")); os.IsNotExist(err) {
			t.Error("File that should have been restored does not exist")
		}

		// The extraneous file should have been deleted
		if _, err := os.Stat(fileToDeletePath); !os.IsNotExist(err) {
			t.Error("Extraneous file was not deleted from the restore directory")
		}
	})

	t.Run("should fail gracefully if an object is missing from the index", func(t *testing.T) {
		// Arrange
		sourceDir := setupRestoreTest(t) // This creates a snap with a few objects
		outputDir := t.TempDir()

		// Find a chunk object hash to remove from the index.
		snaps, err := lib.GetSortedSnaps(sourceDir)
		if err != nil || len(snaps) == 0 {
			t.Fatal("Failed to get snaps to find an object to delete")
		}
		rootTreeHash := snaps[0].RootTreeHash

		rootTree, err := lib.ReadObjectAsJSON[types.Tree](sourceDir, rootTreeHash)
		if err != nil {
			t.Fatalf("Could not read root tree to find a file manifest: %v", err)
		}

		var fileManifestHash string
		for _, entry := range rootTree.Entries {
			if entry.Type == "blob" { // Correctly check for file type
				fileManifestHash = entry.Hash
				break
			}
		}
		if fileManifestHash == "" {
			t.Fatal("Could not find a file manifest in the root tree")
		}

		fileManifest, err := lib.ReadObjectAsJSON[types.FileManifest](sourceDir, fileManifestHash)
		if err != nil {
			t.Fatalf("Could not read file manifest to find a chunk to delete: %v", err)
		}
		if len(fileManifest.Chunks) == 0 {
			t.Fatal("File manifest has no chunks to delete")
		}
		objectToDelete := fileManifest.Chunks[0].Hash

		// Now, corrupt the index by removing this object.
		indexPath := lib.GetIndexPath(sourceDir)
		indexContent, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("Failed to read index file: %v", err)
		}

		var currentIndex types.PackIndex
		if err := json.Unmarshal(indexContent, &currentIndex); err != nil {
			t.Fatalf("Failed to unmarshal index: %v", err)
		}

		delete(currentIndex, objectToDelete) // Remove the object

		// Write the corrupted index back.
		corruptedIndexJSON, err := json.Marshal(currentIndex)
		if err != nil {
			t.Fatalf("Failed to marshal corrupted index: %v", err)
		}
		if err := os.WriteFile(indexPath, corruptedIndexJSON, 0644); err != nil {
			t.Fatalf("Failed to write corrupted index: %v", err)
		}

		// Reset the in-memory state of the object store so it re-reads our corrupted index.
		lib.ResetObjectStoreState()

		// Act
		err = commands.Restore(sourceDir, "1", outputDir)

		// Assert
		if err == nil {
			t.Fatal("Expected restore to fail due to missing object, but it succeeded")
		}
		if !strings.Contains(err.Error(), "not found in index") {
			t.Errorf("Expected error about missing object from index, but got: %v", err)
		}
	})
}
