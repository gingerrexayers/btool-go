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
func markReachableObjects(store *lib.ObjectStore, startHash string, liveHashes *sync.Map) error {
	// Check if we've already processed this hash to avoid redundant work.
	if _, loaded := liveHashes.LoadOrStore(startHash, true); loaded {
		return nil
	}

	// Read the object. We need to determine if it's a tree, a manifest, or a chunk.
	// A simple way is to try to unmarshal it as JSON. Chunks are raw binary and will fail.
	buffer, err := store.ReadObjectAsBuffer(startHash)
	if err != nil {
		return fmt.Errorf("failed to read object %s for marking: %w", startHash, err)
	}

	// Try to unmarshal as a Tree
	var tree types.Tree
	if err := json.Unmarshal(buffer, &tree); err == nil && len(tree.Entries) > 0 {
		for _, entry := range tree.Entries {
			if err := markReachableObjects(store, entry.Hash, liveHashes); err != nil {
				return err
			}
		}
		return nil
	}

	// Try to unmarshal as a FileManifest
	var manifest types.FileManifest
	if err := json.Unmarshal(buffer, &manifest); err == nil && len(manifest.Chunks) > 0 {
		fmt.Printf("  - Scanning manifest %s...\n", startHash)
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
	store := lib.NewObjectStore(absSourceDir)

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
			if err := markReachableObjects(store, s.RootTreeHash, &liveHashes); err != nil {
				errs <- err
			}
		}(snap)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}



	// 3. Sweep Phase: Rebuild the index and copy necessary packfiles.
	fmt.Println("   - Sweeping old objects and rebuilding index...")
	btoolDir := lib.GetBtoolDir(absSourceDir)
	tmpPacksDir := filepath.Join(btoolDir, "packs.tmp")
	_ = os.RemoveAll(tmpPacksDir) // Clean up from previous failed runs
	if err := os.MkdirAll(tmpPacksDir, 0755); err != nil {
		return err
	}

	// Get the current index to find where live objects are stored.
	currentIndex, err := store.GetIndex()
	if err != nil {
		return fmt.Errorf("failed to get current index for sweep: %w", err)
	}

	newIndex := make(types.PackIndex)
	packsToKeep := make(map[string]bool)

	liveHashes.Range(func(key, value interface{}) bool {
		hash := key.(string)
		if entry, exists := currentIndex[hash]; exists {
			newIndex[hash] = entry
			packsToKeep[entry.PackHash] = true
		} else {
			// This case should ideally not happen in a consistent repository.
			// It means a live hash was not found in the index.
			fmt.Fprintf(os.Stderr, "Warning: Live object %s not found in the index during prune.\n", hash)
		}
		return true
	})

	// Copy the required packfiles to the temporary directory.
	packsDir := lib.GetPacksDir(absSourceDir)
	for packHash := range packsToKeep {
		originalPath := filepath.Join(packsDir, packHash)
		newPath := filepath.Join(tmpPacksDir, packHash)
		if err := lib.CopyFile(originalPath, newPath); err != nil {
			return fmt.Errorf("failed to copy packfile %s: %w", packHash, err)
		}
	}

	// 4. Finalization Phase: Write the new index and atomically swap directories.
	fmt.Println("   - Finalizing changes...")
	tmpIndexPath := filepath.Join(btoolDir, "index.tmp.json")
	newIndexJSON, err := json.MarshalIndent(newIndex, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpIndexPath, newIndexJSON, 0644); err != nil {
		return err
	}

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
		// Note: we ignore errors here, as a failure to delete a snap manifest is not critical.
		_ = os.Remove(filepath.Join(snapsDir, snap.Hash+".json"))
	}

	fmt.Println("✅ Prune complete!")
	fmt.Printf("   - Deleted %d old snap(s).\n", len(snapsToPrune))

	return nil
}
