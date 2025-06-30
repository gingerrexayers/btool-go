// Package lib contains the core, reusable services for the btool application.
package lib

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// GetHash calculates the SHA-256 hash of an in-memory byte slice and returns
// it as a lowercase hex-encoded string.
// This is used for hashing content that is already in memory, such as a
// Tree or FileManifest object after it has been serialized to JSON.
func GetHash(content []byte) string {
	hashBytes := sha256.Sum256(content)
	return hex.EncodeToString(hashBytes[:])
}

// GetFileHash calculates the SHA-256 hash of a file's contents by streaming
// it from disk. This is highly memory-efficient as it avoids loading the entire
// file into memory.
// It returns the lowercase hex-encoded hash string and an error if any file
// operation fails.
func GetFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}

	defer file.Close()

	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}
