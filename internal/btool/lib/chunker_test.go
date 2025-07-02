package lib

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFile creates a temporary file with the given content.
// It returns the path to the file and a cleanup function.
func setupTestFile(t *testing.T, content []byte) (string, func()) {
	// Create a temporary directory for the test file.
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "testfile.dat")

	err := os.WriteFile(filePath, content, 0644)
	require.NoError(t, err, "Failed to write test file")

	cleanup := func() {
		// os.RemoveAll(tempDir) is handled by t.TempDir(), so no extra cleanup is needed.
	}

	return filePath, cleanup
}

func TestChunkFile(t *testing.T) {
	t.Run("Chunk a normal-sized file", func(t *testing.T) {
		// Create a file large enough to be chunked into multiple pieces.
		// avgChunkSize is 8KB, so 20KB should produce 2-3 chunks.
		content := make([]byte, 20*1024)
		_, err := rand.Read(content) // Fill with random data
		require.NoError(t, err, "Failed to generate random content")

		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		require.NoError(t, err, "ChunkFile failed with an unexpected error")
		assert.Greater(t, len(chunks), 1, "Expected file to be split into multiple chunks")
		assert.Equal(t, int64(len(content)), totalSize, "Total size should match content length")

		// Verify that the concatenated chunks re-form the original content.
		var reconstructedContent []byte
		for _, chunk := range chunks {
			reconstructedContent = append(reconstructedContent, chunk.Data...)
		}
		assert.Equal(t, content, reconstructedContent, "Reconstructed content should match original")
	})

	t.Run("Chunk a small file (less than min chunk size)", func(t *testing.T) {
		// Create content smaller than the minChunkSize (4KB).
		content := []byte("this file is too small to be split.")
		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		require.NoError(t, err, "ChunkFile failed with an unexpected error")
		// It should be treated as a single chunk.
		require.Len(t, chunks, 1, "Expected 1 chunk for a small file")
		assert.Equal(t, int64(len(content)), totalSize, "Total size should match content length")
		assert.Equal(t, content, chunks[0].Data, "Chunk content should match original file content")
	})

	t.Run("Chunk an empty file", func(t *testing.T) {
		content := []byte{}
		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		require.NoError(t, err, "ChunkFile failed with an unexpected error")
		assert.Empty(t, chunks, "Expected 0 chunks for an empty file")
		assert.Equal(t, int64(0), totalSize, "Expected totalSize to be 0 for an empty file")
	})

	t.Run("Attempt to chunk a non-existent file", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "this_file_does_not_exist.txt")

		_, _, err := ChunkFile(nonExistentPath)

		require.Error(t, err, "Expected an error when chunking a non-existent file")
		// Check that the error is a file system "not exist" error.
		assert.True(t, os.IsNotExist(err), "Expected a 'file not exist' error")
	})

	t.Run("Verify chunk hashes and sizes are correct", func(t *testing.T) {
		content := make([]byte, 10*1024)
		_, err := rand.Read(content)
		require.NoError(t, err, "Failed to generate random content")

		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, _, err := ChunkFile(filePath)
		require.NoError(t, err, "ChunkFile failed")

		for _, chunk := range chunks {
			// Recalculate the hash of the chunk data to ensure it matches.
			expectedHash := GetHash(chunk.Data)
			assert.Equal(t, expectedHash, chunk.Hash, "Chunk hash mismatch")
			// Verify that the stored size matches the actual data length.
			assert.Equal(t, int64(len(chunk.Data)), chunk.Size, "Chunk size mismatch")
		}
	})
}
