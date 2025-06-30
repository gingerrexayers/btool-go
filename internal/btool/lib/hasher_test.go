package lib

import (
	"os"
	"path/filepath"
	"testing"
)



func TestHashing(t *testing.T) {
	// Known SHA-256 hash for the string "hello world"
	const helloWorldHash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	// Known SHA-256 hash for an empty input
	const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	t.Run("GetHash for in-memory content", func(t *testing.T) {
		// Arrange
		content := []byte("hello world")

		// Act
		hash := GetHash(content)

		// Assert
		if hash != helloWorldHash {
			t.Errorf("GetHash() for 'hello world' was incorrect, got: %s, want: %s", hash, helloWorldHash)
		}
	})

	t.Run("GetHash for empty content", func(t *testing.T) {
		// Arrange
		content := []byte{}

		// Act
		hash := GetHash(content)

		// Assert
		if hash != emptyHash {
			t.Errorf("GetHash() for empty content was incorrect, got: %s, want: %s", hash, emptyHash)
		}
	})

	t.Run("GetFileHash for file with content", func(t *testing.T) {
		// Arrange
		filePath, cleanup := setupTestFile(t, []byte("hello world"))
		defer cleanup()

		// Act
		hash, err := GetFileHash(filePath)

		// Assert
		if err != nil {
			t.Fatalf("GetFileHash() failed with an unexpected error: %v", err)
		}
		if hash != helloWorldHash {
			t.Errorf("GetFileHash() for 'hello world' file was incorrect, got: %s, want: %s", hash, helloWorldHash)
		}
	})

	t.Run("GetFileHash for empty file", func(t *testing.T) {
		// Arrange
		filePath, cleanup := setupTestFile(t, []byte{})
		defer cleanup()

		// Act
		hash, err := GetFileHash(filePath)

		// Assert
		if err != nil {
			t.Fatalf("GetFileHash() for empty file failed with an unexpected error: %v", err)
		}
		if hash != emptyHash {
			t.Errorf("GetFileHash() for empty file was incorrect, got: %s, want: %s", hash, emptyHash)
		}
	})

	t.Run("GetFileHash for non-existent file", func(t *testing.T) {
		// Arrange
		nonExistentPath := filepath.Join(t.TempDir(), "this_does_not_exist.txt")

		// Act
		_, err := GetFileHash(nonExistentPath)

		// Assert
		if err == nil {
			t.Fatal("Expected an error when hashing a non-existent file, but got nil")
		}
		if !os.IsNotExist(err) {
			t.Errorf("Expected a 'file not exist' error, but got: %v", err)
		}
	})
}
