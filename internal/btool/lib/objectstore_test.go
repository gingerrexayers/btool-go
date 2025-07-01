package lib

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// setupObjectStoreTest sets up a temporary directory and creates a new ObjectStore instance.
func setupObjectStoreTest(t *testing.T) (*ObjectStore, string) {
	testDir := t.TempDir()

	// Ensure the .btool directory structure exists before running the test.
	if _, err := EnsureBtoolDirs(testDir); err != nil {
		t.Fatalf("Failed to create .btool directories: %v", err)
	}

	store := NewObjectStore(testDir)
	return store, testDir
}

func TestObjectStore(t *testing.T) {
	t.Run("Write, commit, and read a single object", func(t *testing.T) {
		store, testDir := setupObjectStoreTest(t)
		content := []byte("hello object store")
		expectedHash := GetHash(content)

		// Act: Write the object.
		hash, err := store.WriteObject(content)
		if err != nil {
			t.Fatalf("WriteObject failed: %v", err)
		}
		if hash != expectedHash {
			t.Errorf("WriteObject returned incorrect hash: got %s, want %s", hash, expectedHash)
		}

		// Act: Commit the pending changes.
		_, err = store.Commit()
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}

		// Act: Read the object back from the packfile.
		readContent, err := store.ReadObjectAsBuffer(hash)
		if err != nil {
			t.Fatalf("ReadObjectAsBuffer failed: %v", err)
		}

		// Assert
		if !bytes.Equal(content, readContent) {
			t.Error("Read content does not match original content")
		}

		// Assert that the index file was created and is valid
		indexPath := GetIndexPath(testDir)
		indexContent, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("Could not read index file: %v", err)
		}
		var index types.PackIndex
		if err := json.Unmarshal(indexContent, &index); err != nil {
			t.Fatalf("Could not parse index JSON: %v", err)
		}
		if _, ok := index[hash]; !ok {
			t.Errorf("Expected hash %s to be in the index, but it was not", hash)
		}
	})

	t.Run("Read an object from the pending buffer before commit", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("I am pending")

		hash, err := store.WriteObject(content)
		if err != nil {
			t.Fatalf("WriteObject failed: %v", err)
		}

		// Act: Read the object back immediately, without committing.
		readContent, err := store.ReadObjectAsBuffer(hash)
		if err != nil {
			t.Fatalf("ReadObjectAsBuffer failed for pending object: %v", err)
		}

		// Assert
		if !bytes.Equal(content, readContent) {
			t.Error("Read content from pending buffer does not match original")
		}
	})

	t.Run("De-duplicate objects written to the pending buffer", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("write me once")

		// Act
		_, err := store.WriteObject(content)
		if err != nil {
			t.Fatalf("First WriteObject failed: %v", err)
		}
		_, err = store.WriteObject(content) // Write the same content again
		if err != nil {
			t.Fatalf("Second WriteObject failed: %v", err)
		}

		// Assert that only one object is pending
		pendingCount := store.PendingObjectCount()
		if pendingCount != 1 {
			t.Errorf("Expected 1 pending object after de-duplication, got %d", pendingCount)
		}
	})

	t.Run("De-duplicate objects that are already committed", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("already committed")

		// Arrange: Write and commit the object.
		_, _ = store.WriteObject(content)
		_, _ = store.Commit()

		// Act: Write the same object again.
		_, err := store.WriteObject(content)
		if err != nil {
			t.Fatalf("WriteObject for committed object failed: %v", err)
		}

		// Assert: There should now be zero pending objects.
		pendingCount := store.PendingObjectCount()
		if pendingCount != 0 {
			t.Errorf("Expected 0 pending objects when writing an already committed object, got %d", pendingCount)
		}
	})

	t.Run("Return an error when reading a non-existent object", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		nonExistentHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		_, err := store.ReadObjectAsBuffer(nonExistentHash)
		if err == nil {
			t.Fatal("Expected an error when reading non-existent object, but got nil")
		}
	})

	t.Run("Handle concurrent writes without race conditions", func(t *testing.T) {
		store, testDir := setupObjectStoreTest(t)
		numGoroutines := 50
		var wg sync.WaitGroup

		// Act: Spawn many goroutines that write objects concurrently.
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				// Create unique content for each goroutine.
				content := make([]byte, 100)
				_, _ = rand.Read(content)
				_, err := store.WriteObject(content)
				if err != nil {
					// t.Errorf is safe for concurrent use.
					t.Errorf("Concurrent WriteObject failed: %v", err)
				}
			}(i)
		}

		wg.Wait() // Wait for all goroutines to finish.

		// Assert
		pendingCount := store.PendingObjectCount()

		if pendingCount != numGoroutines {
			t.Errorf("Expected %d pending objects after concurrent writes, but got %d", numGoroutines, pendingCount)
		}

		// Commit the results and verify.
		_, err := store.Commit()
		if err != nil {
			t.Fatalf("Commit after concurrent writes failed: %v", err)
		}

		// Check the index size after commit.
		indexPath := GetIndexPath(testDir)
		indexContent, _ := os.ReadFile(indexPath)
		var index types.PackIndex
		_ = json.Unmarshal(indexContent, &index)
		if len(index) != numGoroutines {
			t.Errorf("Expected index to have %d objects after commit, but got %d", numGoroutines, len(index))
		}
	})

	t.Run("Read a JSON object correctly", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		manifest := types.FileManifest{
			Chunks:    []types.ChunkRef{{Hash: GetHash([]byte("c1")), Size: 2}},
			TotalSize: 2,
		}
		manifestJSON, _ := json.Marshal(manifest)

		hash, _ := store.WriteObject(manifestJSON)
		_, _ = store.Commit()

		// Act
		var readManifest types.FileManifest
		err := store.ReadObjectAsJSON(hash, &readManifest)
		if err != nil {
			t.Fatalf("ReadObjectAsJSON failed: %v", err)
		}

		// Assert
		if readManifest.TotalSize != manifest.TotalSize {
			t.Errorf("Read JSON object has wrong TotalSize: got %d, want %d", readManifest.TotalSize, manifest.TotalSize)
		}
		if len(readManifest.Chunks) != 1 || readManifest.Chunks[0].Hash != manifest.Chunks[0].Hash {
			t.Error("Read JSON object has incorrect chunk data")
		}
	})
}
