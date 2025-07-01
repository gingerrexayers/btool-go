package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
)

// helper function to create a temporary directory with a file
func createTestRepo(t *testing.T, content string) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "btool-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	if content != "" {
		if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Return the directory path and a cleanup function
	return tmpDir, func() { os.RemoveAll(tmpDir) }
}

func TestSnapWithMessage(t *testing.T) {
	tmpDir, cleanup := createTestRepo(t, "hello world")
	defer cleanup()

	// 1. Create a snap with a message
	message := "this is a test message"
	if err := Snap(tmpDir, message); err != nil {
		t.Fatalf("Snap() failed: %v", err)
	}

	// 2. Get the snaps and check the message
	snaps, err := lib.GetSortedSnaps(tmpDir)
	if err != nil {
		t.Fatalf("GetSortedSnaps() failed: %v", err)
	}

	if len(snaps) != 1 {
		t.Fatalf("Expected 1 snap, got %d", len(snaps))
	}

	if snaps[0].Message != message {
		t.Errorf("Expected message '%s', got '%s'", message, snaps[0].Message)
	}
}

func TestListDoesNotResetState(t *testing.T) {
	tmpDir, cleanup := createTestRepo(t, "file1")
	defer cleanup()

	// 1. Create an initial snap
	if err := Snap(tmpDir, "first snap"); err != nil {
		t.Fatalf("Snap() failed: %v", err)
	}

	// 2. Get the initial index state
	store := lib.NewObjectStore(tmpDir)
	index1, err := store.GetIndex()
	if err != nil {
		t.Fatalf("GetIndex() failed: %v", err)
	}
	if len(index1) == 0 {
		t.Fatal("Index is empty after first snap, should not be")
	}

	// 3. Run the List command
	if err := List(tmpDir); err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	// 4. Get the index state again and compare
	// Re-create the store to simulate a separate command run
	store2 := lib.NewObjectStore(tmpDir)
	index2, err := store2.GetIndex()
	if err != nil {
		t.Fatalf("GetIndex() failed after List(): %v", err)
	}

	if len(index1) != len(index2) {
		t.Errorf("List command appears to have reset the state. Index size before: %d, after: %d", len(index1), len(index2))
	}
}
