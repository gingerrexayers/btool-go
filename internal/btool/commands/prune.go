package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// PruneOptions holds the configuration for the prune command.
type PruneOptions struct {
	SnapIdentifier string
}

// markReachableObjects is a recursive function to find all objects referenced by a starting hash.
// It's designed to be run in a goroutine.
func markReachableObjects(baseDir, startHash string, liveHashes *sync.Map) error {
	// Check if we've already processed this hash to avoid redundant work.
	if _, loaded := liveHashes.LoadOrStore(startHash, true); loaded {
		return nil
	}

	// Read the object. We need to determine if it's a tree, a manifest, or a chunk.
	// A simple way is to try to unmarshal it as JSON. Chunks are raw binary and will fail.
	buffer, err := lib.ReadObjectAsBuffer(baseDir, startHash)
	if err != nil {
		return fmt.Errorf("failed to read object %s for marking: %w", startHash, err)
	}

	// Try to unmarshal as a Tree
	var tree types.Tree
	if err := json.Unmarshal(buffer, &tree); err == nil && len(tree.Entries) > 0 {
		for _, entry := range tree.Entries {
			if err := markReachableObjects(baseDir, entry.Hash, liveHashes); err != nil {
				return err
			}
		}
		return nil
	}

	// Try to unmarshal as a FileManifest
	var manifest types.FileManifest
	if err := json.Unmarshal(buffer, &manifest); err == nil && len(manifest.Chunks) > 0 {
		for _, chunk := range manifest.Chunks {
			liveHashes.Store(chunk.Hash, true) // Chunks are leaves in the graph.
		}
		return nil
	}

	// If it's not a valid Tree or Manifest, we assume it's a chunk, which is already marked.
	return nil
}

// Prune is the main function for the 'prune' command.
func Prune(directory string, options PruneOptions) error {
	absSourceDir, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Errorf("could not resolve path: %w", err)
	}

	fmt.Printf("🧹 Starting prune for \"%s\", removing snaps older than %s...\n", absSourceDir, options.SnapIdentifier)
	lib.ResetObjectStoreState()

	// 1. Identify Snaps to Keep and Prune
	allSnaps, err := lib.GetSortedSnaps(absSourceDir)
	if err != nil {
		return fmt.Errorf("could not get snapshots: %w", err)
	}

	// Find the snapshot to prune from.
	snapToKeepFrom, err := lib.FindSnap(absSourceDir, options.SnapIdentifier)
	if err != nil {
		return fmt.Errorf("failed to find snapshot %s: %w", options.SnapIdentifier, err)
	}

	// Find the index of the snapshot in the sorted list (oldest to newest).
	keepFromIndex := -1
	for i, s := range allSnaps {
		if s.Hash == snapToKeepFrom.Hash {
			keepFromIndex = i
			break
		}
	}
	if keepFromIndex == -1 {
		return fmt.Errorf("internal error: could not find specified snapshot in the timeline")
	}

	snapsToKeep := allSnaps[keepFromIndex:]
	snapsToPrune := allSnaps[:keepFromIndex]

	if len(snapsToPrune) == 0 {
		fmt.Println("No snapshots older than the specified one to prune.")
		return nil
	}

	// 2. Mark Phase
	fmt.Println("   - Marking live objects from snapshots to keep...")
	var liveHashes sync.Map // A thread-safe map
	var wg sync.WaitGroup
	errs := make(chan error, len(snapsToKeep))

	for _, snap := range snapsToKeep {
		wg.Add(1)
		go func(s lib.SnapDetail) {
			defer wg.Done()
			if err := markReachableObjects(absSourceDir, s.RootTreeHash, &liveHashes); err != nil {
				errs <- err
			}
		}(snap)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return fmt.Errorf("an error occurred during the mark phase: %w", err)
		}
	}

	// 3. Sweep (Repack) Phase
	fmt.Println("   - Repacking live objects into new packfiles...")
	btoolDir := lib.GetBtoolDir(absSourceDir)
	tmpPacksDir := filepath.Join(btoolDir, "packs.tmp")
	_ = os.RemoveAll(tmpPacksDir) // Clean up from previous failed runs
	if err := os.MkdirAll(tmpPacksDir, 0755); err != nil {
		return err
	}

	newIndex := make(types.PackIndex)
	var newPackBuffer []byte
	var newPackOffset int64 = 0

	// Iterate over the thread-safe map
	liveHashes.Range(func(key, value interface{}) bool {
		hash := key.(string)
		buffer, err := lib.ReadObjectAsBuffer(absSourceDir, hash)
		if err != nil {
			// This is a critical error, something is wrong with the object store.
			fmt.Fprintf(os.Stderr, "Warning: could not read object %s during repack: %v\n", hash, err)
			return true // continue iteration
		}
		newIndex[hash] = types.PackIndexEntry{ /* PackHash is set later */ Offset: newPackOffset, Length: int64(len(buffer))}
		newPackBuffer = append(newPackBuffer, buffer...)
		newPackOffset += int64(len(buffer))
		return true
	})

	if len(newPackBuffer) > 0 {
		newPackHash := lib.GetHash(newPackBuffer)
		// Update all new entries with the correct pack hash
		for hash, entry := range newIndex {
			entry.PackHash = newPackHash
			newIndex[hash] = entry
		}
		if err := os.WriteFile(filepath.Join(tmpPacksDir, newPackHash), newPackBuffer, 0644); err != nil {
			return err
		}
	}

	// 4. Atomic Swap
	fmt.Println("   - Finalizing changes...")
	tmpIndexPath := filepath.Join(btoolDir, "index.tmp.json")
	indexJSON, _ := json.MarshalIndent(newIndex, "", "  ")
	if err := os.WriteFile(tmpIndexPath, indexJSON, 0644); err != nil {
		return err
	}

	packsDir := lib.GetPacksDir(absSourceDir)
	indexPath := lib.GetIndexPath(absSourceDir)
	bakPacksDir := packsDir + ".bak"
	bakIndexPath := indexPath + ".bak"

	_ = os.RemoveAll(bakPacksDir) // Remove old backup if it exists
	_ = os.Remove(bakIndexPath)

	if err := os.Rename(packsDir, bakPacksDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to backup old packs directory: %w", err)
	}
	if err := os.Rename(indexPath, bakIndexPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to backup old index file: %w", err)
	}

	if err := os.Rename(tmpPacksDir, packsDir); err != nil {
		return fmt.Errorf("failed to activate new packs directory: %w", err)
	}
	if err := os.Rename(tmpIndexPath, indexPath); err != nil {
		return fmt.Errorf("failed to activate new index file: %w", err)
	}

	_ = os.RemoveAll(bakPacksDir)
	_ = os.Remove(bakIndexPath)

	// 5. Cleanup old snapshot manifests
	snapsDir := lib.GetSnapsDir(absSourceDir)
	for _, snap := range snapsToPrune {
		if err := os.Remove(filepath.Join(snapsDir, snap.Hash+".json")); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not delete old snap manifest %s: %v\n", snap.Hash, err)
		}
	}

	fmt.Println("✅ Prune complete!")
	fmt.Printf("   - Deleted %d old snap(s).\n", len(snapsToPrune))

	return nil
}
