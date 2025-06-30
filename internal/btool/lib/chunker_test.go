package lib

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// setupTestFile creates a temporary file with the given content.
// It returns the path to the file and a cleanup function.
func setupTestFile(t *testing.T, content []byte) (string, func()) {
	// Create a temporary directory for the test file.
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "testfile.dat")

	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

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
		if err != nil {
			t.Fatalf("Failed to generate random content: %v", err)
		}

		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		if err != nil {
			t.Fatalf("ChunkFile failed with an unexpected error: %v", err)
		}
		if len(chunks) <= 1 {
			t.Errorf("Expected file to be split into multiple chunks, but got %d", len(chunks))
		}
		if totalSize != int64(len(content)) {
			t.Errorf("Expected totalSize to be %d, but got %d", len(content), totalSize)
		}

		// Verify that the concatenated chunks re-form the original content.
		var reconstructedContent []byte
		for _, chunk := range chunks {
			reconstructedContent = append(reconstructedContent, chunk.Data...)
		}
		if !bytes.Equal(content, reconstructedContent) {
			t.Error("Reconstructed content does not match original file content")
		}
	})

	t.Run("Chunk a small file (less than min chunk size)", func(t *testing.T) {
		// Create content smaller than the minChunkSize (4KB).
		content := []byte("this file is too small to be split.")
		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		if err != nil {
			t.Fatalf("ChunkFile failed with an unexpected error: %v", err)
		}
		// It should be treated as a single chunk.
		if len(chunks) != 1 {
			t.Errorf("Expected 1 chunk for a small file, but got %d", len(chunks))
		}
		if totalSize != int64(len(content)) {
			t.Errorf("Expected totalSize to be %d, but got %d", len(content), totalSize)
		}
		if !bytes.Equal(content, chunks[0].Data) {
			t.Error("Chunk content does not match original file content")
		}
	})

	t.Run("Chunk an empty file", func(t *testing.T) {
		content := []byte{}
		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, totalSize, err := ChunkFile(filePath)

		if err != nil {
			t.Fatalf("ChunkFile failed with an unexpected error: %v", err)
		}
		if len(chunks) != 0 {
			t.Errorf("Expected 0 chunks for an empty file, but got %d", len(chunks))
		}
		if totalSize != 0 {
			t.Errorf("Expected totalSize to be 0 for an empty file, but got %d", totalSize)
		}
	})

	t.Run("Attempt to chunk a non-existent file", func(t *testing.T) {
		nonExistentPath := filepath.Join(t.TempDir(), "this_file_does_not_exist.txt")

		_, _, err := ChunkFile(nonExistentPath)

		if err == nil {
			t.Fatal("Expected an error when chunking a non-existent file, but got nil")
		}
		// Check that the error is a file system "not exist" error.
		if !os.IsNotExist(err) {
			t.Errorf("Expected a 'file not exist' error, but got a different error: %v", err)
		}
	})

	t.Run("Verify chunk hashes and sizes are correct", func(t *testing.T) {
		content := make([]byte, 10*1024)
		_, err := rand.Read(content)
		if err != nil {
			t.Fatalf("Failed to generate random content: %v", err)
		}

		filePath, cleanup := setupTestFile(t, content)
		defer cleanup()

		chunks, _, err := ChunkFile(filePath)
		if err != nil {
			t.Fatalf("ChunkFile failed: %v", err)
		}

		for _, chunk := range chunks {
			// Recalculate the hash of the chunk data to ensure it matches.
			expectedHash := GetHash(chunk.Data)
			if chunk.Hash != expectedHash {
				t.Errorf("Chunk hash mismatch: expected %s, got %s", expectedHash, chunk.Hash)
			}
			// Verify that the stored size matches the actual data length.
			if chunk.Size != int64(len(chunk.Data)) {
				t.Errorf("Chunk size mismatch: expected %d, got %d", len(chunk.Data), chunk.Size)
			}
		}
	})
}
