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

// ObjectStore manages all interactions with the underlying data store,
// including packfiles and the central index. It is designed to be instantiated
// once per command execution to ensure state isolation.
type ObjectStore struct {
	baseDir        string
	mutex          sync.Mutex
	packIndex      types.PackIndex
	pendingObjects map[string][]byte
	indexLoaded    bool
}

// NewObjectStore creates and initializes a new ObjectStore for a given repository.
func NewObjectStore(baseDir string) *ObjectStore {
	return &ObjectStore{
		baseDir:        baseDir,
		pendingObjects: make(map[string][]byte),
		packIndex:      make(types.PackIndex),
	}
}

// loadIndex reads the index.json file into the in-memory cache.
// It is NOT thread-safe by itself and should be called from within a locked section.
func (s *ObjectStore) loadIndex() error {
	if s.indexLoaded {
		return nil
	}

	indexPath := GetIndexPath(s.baseDir)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		// Index doesn't exist yet, which is fine. The in-memory index is already empty.
		s.indexLoaded = true
		return nil
	}

	content, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(content, &s.packIndex); err != nil {
		return err
	}

	s.indexLoaded = true
	return nil
}

// WriteObject adds an object to the in-memory pending buffer.
// The object is not persisted to disk until Commit() is called.
func (s *ObjectStore) WriteObject(data []byte) (string, error) {
	hash := GetHash(data)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.loadIndex(); err != nil {
		return "", err
	}

	// De-duplication check:
	if _, exists := s.packIndex[hash]; exists {
		return hash, nil
	}
	if _, exists := s.pendingObjects[hash]; exists {
		return hash, nil
	}

	s.pendingObjects[hash] = data
	return hash, nil
}

// Commit writes all pending objects to a new single packfile on disk
// and updates the index.json file to make them persistent.
func (s *ObjectStore) Commit() (int64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.pendingObjects) == 0 {
		return 0, nil // Nothing to commit.
	}

	var hashes []string
	for hash := range s.pendingObjects {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)

	var packBuffer []byte
	var currentOffset int64 = 0
	newEntries := make(map[string]types.PackIndexEntry)

	for _, hash := range hashes {
		data := s.pendingObjects[hash]
		packBuffer = append(packBuffer, data...)
		newEntries[hash] = types.PackIndexEntry{
			Offset: currentOffset,
			Length: int64(len(data)),
		}
		currentOffset += int64(len(data))
	}

	packHash := GetHash(packBuffer)
	packsDir := GetPacksDir(s.baseDir)
	packPath := filepath.Join(packsDir, packHash)
	if err := os.WriteFile(packPath, packBuffer, 0644); err != nil {
		return 0, err
	}

	if err := s.loadIndex(); err != nil {
		return 0, err
	}

	for hash, entry := range newEntries {
		entry.PackHash = packHash
		s.packIndex[hash] = entry
	}

	indexPath := GetIndexPath(s.baseDir)
	indexJSON, err := json.MarshalIndent(s.packIndex, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(indexPath, indexJSON, 0644); err != nil {
		return 0, err
	}

	s.pendingObjects = make(map[string][]byte)

	return int64(len(packBuffer)), nil
}

// ReadObjectAsBuffer retrieves an object from the store by its hash.
func (s *ObjectStore) ReadObjectAsBuffer(hash string) ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if data, exists := s.pendingObjects[hash]; exists {
		return data, nil
	}

	if err := s.loadIndex(); err != nil {
		return nil, err
	}

	entry, exists := s.packIndex[hash]
	if !exists {
		return nil, errors.New("object with hash " + hash + " not found in index")
	}

	packPath := filepath.Join(GetPacksDir(s.baseDir), entry.PackHash)
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
func (s *ObjectStore) ReadObjectAsJSON(hash string, target interface{}) error {
	buffer, err := s.ReadObjectAsBuffer(hash)
	if err != nil {
		return err
	}
	return json.Unmarshal(buffer, target)
}

// GetIndex returns a copy of the current pack index.
func (s *ObjectStore) GetIndex() (types.PackIndex, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := s.loadIndex(); err != nil {
		return nil, err
	}

	indexCopy := make(types.PackIndex)
	for hash, entry := range s.packIndex {
		indexCopy[hash] = entry
	}

	return indexCopy, nil
}

// PendingObjectCount returns the number of objects waiting to be committed.
// This is intended for use in tests.
func (s *ObjectStore) PendingObjectCount() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.pendingObjects)
}

// ResetObjectStoreState is a helper for testing. It clears the in-memory caches.
// This is no longer needed with the ObjectStore struct approach, but is kept
// for any legacy tests that have not been updated.
func ResetObjectStoreState() {
	// This function is now a no-op but is kept for backward compatibility
	// with any tests that might still call it.
}
