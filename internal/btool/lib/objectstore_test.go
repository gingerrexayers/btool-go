package lib

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupObjectStoreTest sets up a temporary directory and creates a new ObjectStore instance.
func setupObjectStoreTest(t *testing.T) (*ObjectStore, string) {
	t.Helper()
	testDir := t.TempDir()

	// Ensure the .btool directory structure exists before running the test.
	_, err := EnsureBtoolDirs(testDir)
	require.NoError(t, err, "Failed to create .btool directories")

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
		require.NoError(t, err, "WriteObject failed")
		assert.Equal(t, expectedHash, hash, "WriteObject returned incorrect hash")

		// Act: Commit the pending changes.
		_, err = store.Commit()
		require.NoError(t, err, "Commit failed")

		// Act: Read the object back from the packfile.
		readContent, err := store.ReadObjectAsBuffer(hash)
		require.NoError(t, err, "ReadObjectAsBuffer failed")

		// Assert
		assert.Equal(t, content, readContent, "Read content does not match original content")

		// Assert that the index file was created and is valid
		indexPath := GetIndexPath(testDir)
		indexContent, err := os.ReadFile(indexPath)
		require.NoError(t, err, "Could not read index file")

		var index types.PackIndex
		err = json.Unmarshal(indexContent, &index)
		require.NoError(t, err, "Could not parse index JSON")
		assert.Contains(t, index, hash, "Expected hash to be in the index")
	})

	t.Run("Read an object from the pending buffer before commit", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("I am pending")

		hash, err := store.WriteObject(content)
		require.NoError(t, err, "WriteObject failed")

		// Act: Read the object back immediately, without committing.
		readContent, err := store.ReadObjectAsBuffer(hash)
		require.NoError(t, err, "ReadObjectAsBuffer failed for pending object")

		// Assert
		assert.Equal(t, content, readContent, "Read content from pending buffer does not match original")
	})

	t.Run("De-duplicate objects written to the pending buffer", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("write me once")

		// Act
		_, err := store.WriteObject(content)
		require.NoError(t, err, "First WriteObject failed")

		_, err = store.WriteObject(content) // Write the same content again
		require.NoError(t, err, "Second WriteObject failed")

		// Assert that only one object is pending
		pendingCount := store.PendingObjectCount()
		assert.Equal(t, 1, pendingCount, "Expected 1 pending object after de-duplication")
	})

	t.Run("De-duplicate objects that are already committed", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		content := []byte("already committed")

		// Arrange: Write and commit the object.
		_, err := store.WriteObject(content)
		require.NoError(t, err)
		_, err = store.Commit()
		require.NoError(t, err)

		// Act: Write the same object again.
		_, err = store.WriteObject(content)
		require.NoError(t, err, "WriteObject for committed object failed")

		// Assert: There should now be zero pending objects.
		pendingCount := store.PendingObjectCount()
		assert.Equal(t, 0, pendingCount, "Expected 0 pending objects when writing an already committed object")
	})

	t.Run("Return an error when reading a non-existent object", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		nonExistentHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		_, err := store.ReadObjectAsBuffer(nonExistentHash)
		require.Error(t, err, "Expected an error when reading non-existent object, but got nil")
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
				_, err := rand.Read(content)
				require.NoError(t, err) // Use require inside goroutine for immediate failure feedback

				_, err = store.WriteObject(content)
				assert.NoError(t, err, "Concurrent WriteObject failed")
			}(i)
		}

		wg.Wait() // Wait for all goroutines to finish.

		// Assert
		pendingCount := store.PendingObjectCount()
		assert.Equal(t, numGoroutines, pendingCount, "Expected %d pending objects after concurrent writes", numGoroutines)

		// Commit the results and verify.
		_, err := store.Commit()
		require.NoError(t, err, "Commit after concurrent writes failed")

		// Check the index size after commit.
		indexPath := GetIndexPath(testDir)
		indexContent, err := os.ReadFile(indexPath)
		require.NoError(t, err)

		var index types.PackIndex
		err = json.Unmarshal(indexContent, &index)
		require.NoError(t, err)
		assert.Equal(t, numGoroutines, len(index), "Expected index to have %d objects after commit", numGoroutines)
	})

	t.Run("Read a JSON object correctly", func(t *testing.T) {
		store, _ := setupObjectStoreTest(t)
		manifest := types.FileManifest{
			Chunks:    []types.ChunkRef{{Hash: GetHash([]byte("c1")), Size: 2}},
			TotalSize: 2,
		}
		manifestJSON, err := json.Marshal(manifest)
		require.NoError(t, err)

		hash, err := store.WriteObject(manifestJSON)
		require.NoError(t, err)
		_, err = store.Commit()
		require.NoError(t, err)

		// Act
		var readManifest types.FileManifest
		err = store.ReadObjectAsJSON(hash, &readManifest)
		require.NoError(t, err, "ReadObjectAsJSON failed")

		// Assert
		assert.Equal(t, manifest.TotalSize, readManifest.TotalSize, "Read JSON object has wrong TotalSize")
		assert.Equal(t, manifest.Chunks, readManifest.Chunks, "Read JSON object has incorrect chunk data")
	})
}
