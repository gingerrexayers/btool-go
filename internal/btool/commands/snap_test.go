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
)

// setupTestDir is a helper function to create a temporary directory structure for testing.
// It returns the root path of the test directory. Using t.TempDir() ensures automatic cleanup.
func setupTestDir(t *testing.T) string {
	// Reset global ignore state before each test run to ensure isolation.
	lib.ResetIgnoreState()

	testDir := t.TempDir()

	// Create a nested structure
	if err := os.Mkdir(filepath.Join(testDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create test files: two unique, two identical for de-duplication testing.
	if err := os.WriteFile(filepath.Join(testDir, "fileA.txt"), []byte("unique content A"), 0644); err != nil {
		t.Fatalf("Failed to write fileA.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "fileB.txt"), []byte("identical content"), 0644); err != nil {
		t.Fatalf("Failed to write fileB.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "subdir", "fileC.txt"), []byte("identical content"), 0644); err != nil {
		t.Fatalf("Failed to write subdir/fileC.txt: %v", err)
	}

	// Create an ignored file and the .btoolignore file to test the ignore logic.
	ignoreContent := "# Ignore log files via glob\n*.log\n\n# Ignore a specific directory\nignored_dir/"
	if err := os.WriteFile(filepath.Join(testDir, ".btoolignore"), []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to write .btoolignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "app.log"), []byte("this log should be ignored"), 0644); err != nil {
		t.Fatalf("Failed to write app.log: %v", err)
	}
	if err := os.Mkdir(filepath.Join(testDir, "ignored_dir"), 0755); err != nil {
		t.Fatalf("Failed to create ignored_dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "ignored_dir", "some_file.txt"), []byte("this should also be ignored"), 0644); err != nil {
		t.Fatalf("Failed to write to ignored_dir: %v", err)
	}

	return testDir
}

// TestSnapCommand is a comprehensive integration test for the public Snap() function.
func TestSnapCommand(t *testing.T) {
	// 1. Setup the test environment.
	testDir := setupTestDir(t)

	// 2. Act: Call the public Snap function from the 'commands' package.
	err := commands.Snap(testDir, "My first integration test snap")
	if err != nil {
		t.Fatalf("commands.Snap() failed unexpectedly: %v", err)
	}

	// 3. Assert - Check the filesystem state after the command has run.
	snapsDir := lib.GetSnapsDir(testDir)
	packsDir := lib.GetPacksDir(testDir)

	// Check if essential directories and files were created.
	if _, err := os.Stat(snapsDir); os.IsNotExist(err) {
		t.Fatal(".btool/snaps directory was not created")
	}
	if _, err := os.Stat(packsDir); os.IsNotExist(err) {
		t.Fatal(".btool/packs directory was not created")
	}

	// There should be exactly one snapshot manifest.
	snapFiles, err := os.ReadDir(snapsDir)
	if err != nil {
		t.Fatalf("Could not read snaps directory: %v", err)
	}
	if len(snapFiles) != 1 {
		t.Fatalf("Expected 1 snapshot file, but found %d", len(snapFiles))
	}

	// There should be exactly one pack file containing all objects.
	packFiles, err := os.ReadDir(packsDir)
	if err != nil {
		t.Fatalf("Could not read packs directory: %v", err)
	}
	if len(packFiles) != 1 {
		t.Fatalf("Expected 1 pack file, but found %d", len(packFiles))
	}

	// 4. Assert - Deeply inspect the contents of the created snapshot.
	snapFileName := snapFiles[0].Name()
	snapContent, err := os.ReadFile(filepath.Join(snapsDir, snapFileName))
	if err != nil {
		t.Fatalf("Could not read snapshot file: %v", err)
	}

	var snapData types.Snap
	if err := json.Unmarshal(snapContent, &snapData); err != nil {
		t.Fatalf("Could not parse snapshot JSON: %v", err)
	}

	// Verify snapshot metadata.
	if snapData.Message != "My first integration test snap" {
		t.Errorf("Expected snap message 'My first integration test snap', got '%s'", snapData.Message)
	}
	// Total size of "unique content A" (16) + "identical content" (17) * 2 = 50
	expectedSize := int64(len("unique content A") + len("identical content")*2)
	if snapData.SourceSize != expectedSize {
		t.Errorf("Expected source size %d, got %d", expectedSize, snapData.SourceSize)
	}

	// Read and verify the root tree from the object store.
	store := lib.NewObjectStore(testDir)
	var rootTree types.Tree
	if err := store.ReadObjectAsJSON(snapData.RootTreeHash, &rootTree); err != nil {
		t.Fatalf("Could not read root tree object from store: %v", err)
	}

	// The root tree should contain 2 files and 1 directory. 'app.log' and 'ignored_dir' must NOT be present.
	if len(rootTree.Entries) != 3 {
		t.Fatalf("Expected root tree to have 3 entries (fileA, fileB, subdir), got %d", len(rootTree.Entries))
	}

	var fileA, fileB, subDirEntry types.TreeEntry
	for _, entry := range rootTree.Entries {
		switch entry.Name {
		case "fileA.txt":
			fileA = entry
		case "fileB.txt":
			fileB = entry
		case "subdir":
			subDirEntry = entry
		}
	}
	if fileA.Name == "" || fileB.Name == "" || subDirEntry.Name == "" {
		t.Fatal("Root tree is missing one or more expected file/dir entries")
	}
	if subDirEntry.Type != "tree" {
		t.Errorf("Expected 'subdir' entry to be of type 'tree', got '%s'", subDirEntry.Type)
	}

	// Read and verify the sub-tree for the 'subdir' directory.
	var subTree types.Tree
	if err := store.ReadObjectAsJSON(subDirEntry.Hash, &subTree); err != nil {
		t.Fatalf("Could not read sub-tree object: %v", err)
	}
	if len(subTree.Entries) != 1 || subTree.Entries[0].Name != "fileC.txt" {
		t.Fatalf("Sub-tree has incorrect entries, expected only 'fileC.txt'")
	}

	// 5. Assert - De-duplication of content.
	// The manifest hash for fileB ("identical content") must be the same as fileC ("identical content").
	fileC := subTree.Entries[0]
	if fileB.Hash != fileC.Hash {
		t.Errorf("De-duplication failed: fileB hash (%s) does not match fileC hash (%s) for identical content", fileB.Hash, fileC.Hash)
	}
	// And they must be different from fileA ("unique content A").
	if fileA.Hash == fileB.Hash {
		t.Error("fileA hash should not match fileB hash for different content")
	}
}

func TestSnapCommand_EmptyDir(t *testing.T) {
	// Arrange: Create an empty directory.
	lib.ResetIgnoreState()
	testDir := t.TempDir()

	// Act: Take a snapshot of the empty directory.
	if err := commands.Snap(testDir, "empty dir snap"); err != nil {
		t.Fatalf("Snap command failed for an empty directory: %v", err)
	}

	// Assert: A snapshot was created.
	snaps, err := lib.GetSortedSnaps(testDir)
	if err != nil {
		t.Fatalf("Failed to get snaps after snapshotting empty dir: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("Expected 1 snapshot for empty dir, but found %d", len(snaps))
	}
	if snaps[0].Message != "empty dir snap" {
		t.Errorf("Incorrect snap message: got '%s', want 'empty dir snap'", snaps[0].Message)
	}

	// Assert: The root tree is empty.
	store := lib.NewObjectStore(testDir)
	var rootTree types.Tree
	if err := store.ReadObjectAsJSON(snaps[0].RootTreeHash, &rootTree); err != nil {
		t.Fatalf("Failed to read root tree of empty snap: %v", err)
	}
	if len(rootTree.Entries) != 0 {
		t.Errorf("Expected root tree to be empty, but it has %d entries", len(rootTree.Entries))
	}

	// Act: Restore the snapshot to a new directory.
	outputDir := t.TempDir()
	if err := commands.Restore(testDir, snaps[0].Hash, outputDir); err != nil {
		t.Fatalf("Failed to restore empty snapshot: %v", err)
	}

	// Assert: The restored directory is empty.
	files, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("Could not read restored directory: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Restored directory is not empty, contains %d files", len(files))
	}
}
