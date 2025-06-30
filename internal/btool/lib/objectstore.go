// Package lib contains the core, reusable services for the btool application.
package lib

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// --- Package-level State ---

// packIndex is an in-memory cache of the index.json file.
// It maps an object's hash to its location in a packfile.
var packIndex types.PackIndex

// pendingObjects holds new objects that have been written but not yet
// committed to a packfile. It maps an object's hash to its raw data.
var pendingObjects = make(map[string][]byte)

// stateMutex is a mutex to protect access to the shared package-level state
// (packIndex and pendingObjects), ensuring thread safety.
var stateMutex = &sync.Mutex{}

// indexLoaded is a flag to prevent redundant reads of the index file.
var indexLoaded = false

// --- Core Functions ---

// loadIndex reads the index.json file into the in-memory cache.
// It is NOT thread-safe by itself and should be called from within a
// locked section.
func loadIndex(baseDir string) error {
	if indexLoaded {
		return nil
	}

	indexPath := GetIndexPath(baseDir)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		// Index doesn't exist yet, initialize an empty one.
		packIndex = make(types.PackIndex)
		indexLoaded = true
		return nil
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(content, &packIndex); err != nil {
		return err
	}

	indexLoaded = true
	return nil
}

// WriteObject adds an object to the in-memory pending buffer.
// The object is not persisted to disk until Commit() is called.
// This function is thread-safe.
func WriteObject(baseDir string, data []byte) (string, error) {
	hash := GetHash(data)

	stateMutex.Lock()
	defer stateMutex.Unlock()

	// Ensure the index is loaded before we check it.
	if err := loadIndex(baseDir); err != nil {
		return "", err
	}

	// De-duplication check:
	// 1. Check if the object is already persisted in a packfile.
	if _, exists := packIndex[hash]; exists {
		return hash, nil
	}
	// 2. Check if the object is already pending to be written.
	if _, exists := pendingObjects[hash]; exists {
		return hash, nil
	}

	// Add the object to the pending map. We copy the data to ensure
	// the caller can't modify the buffer after we've taken it.
	pendingObjects[hash] = data
	return hash, nil
}

// Commit writes all pending objects to a new single packfile on disk
// and updates the index.json file to make them persistent.
// This function is thread-safe.
func Commit(baseDir string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	if len(pendingObjects) == 0 {
		return nil // Nothing to commit.
	}

	// 1. Concatenate all pending objects into a single buffer for the new packfile.
	// We sort the hashes to ensure the packfile is deterministic, which is good practice.
	var hashes []string
	for hash := range pendingObjects {
		hashes = append(hashes, hash)
	}
	// Sort the hashes for deterministic packfile ordering
	sort.Strings(hashes)

	var packBuffer []byte
	var currentOffset int64 = 0
	packHash := "" // Will be calculated after buffer is built

	// We need a temporary map to build the new index entries before we know the pack hash.
	newEntries := make(map[string]types.PackIndexEntry)

	for _, hash := range hashes {
		data := pendingObjects[hash]
		packBuffer = append(packBuffer, data...)
		newEntries[hash] = types.PackIndexEntry{
			// PackHash is empty for now
			Offset: currentOffset,
			Length: int64(len(data)),
		}
		currentOffset += int64(len(data))
	}

	// 2. Calculate the hash of the new packfile and write it to disk.
	packHash = GetHash(packBuffer)
	packsDir := GetPacksDir(baseDir)
	packPath := filepath.Join(packsDir, packHash)
	if err := os.WriteFile(packPath, packBuffer, 0644); err != nil {
		return err
	}

	// 3. Update the packIndex with the new entries.
	if err := loadIndex(baseDir); err != nil {
		return err // Ensure index is loaded before modifying
	}
	for hash, entry := range newEntries {
		entry.PackHash = packHash // Now we know the pack hash
		packIndex[hash] = entry
	}

	// 4. Write the updated index back to disk.
	indexPath := GetIndexPath(baseDir)
	indexJSON, err := json.MarshalIndent(packIndex, "", "  ") // Pretty-print JSON
	if err != nil {
		return err
	}
	if err := os.WriteFile(indexPath, indexJSON, 0644); err != nil {
		return err
	}

	// 5. Clear the pending objects map as they are now committed.
	pendingObjects = make(map[string][]byte)

	return nil
}

// ReadObjectAsBuffer retrieves an object from the store by its hash.
// It first checks pending objects, then consults the index to read from a packfile.
// This function is thread-safe.
func ReadObjectAsBuffer(baseDir, hash string) ([]byte, error) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	// Check pending objects first.
	if data, exists := pendingObjects[hash]; exists {
		return data, nil
	}

	if err := loadIndex(baseDir); err != nil {
		return nil, err
	}

	entry, exists := packIndex[hash]
	if !exists {
		return nil, errors.New("object with hash " + hash + " not found in index")
	}

	packPath := filepath.Join(GetPacksDir(baseDir), entry.PackHash)

	// Here we must be careful with file I/O. We open the file, seek to the
	// correct offset, and read the specified number of bytes.
	file, err := os.Open(packPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, entry.Length)
	_, err = file.ReadAt(buffer, entry.Offset)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

// ReadObjectAsJSON retrieves an object and unmarshals it into a given struct.
func ReadObjectAsJSON[T any](baseDir, hash string) (*T, error) {
	buffer, err := ReadObjectAsBuffer(baseDir, hash)
	if err != nil {
		return nil, err
	}

	var target T
	if err := json.Unmarshal(buffer, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

// ResetObjectStoreState is a helper for testing. It clears the in-memory caches.
func ResetObjectStoreState() {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	packIndex = nil
	pendingObjects = make(map[string][]byte)
	indexLoaded = false
}
