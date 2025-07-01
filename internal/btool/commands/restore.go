package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// fileRestoreJob holds the information needed for a worker to restore one file.
type fileRestoreJob struct {
	ManifestHash    string
	DestinationPath string
	Mode            os.FileMode
}

// restoreFileWorker is the logic executed by each goroutine in the pool.
// It reads jobs from a channel, restores the file, and signals completion.
func restoreFileWorker(wg *sync.WaitGroup, store *lib.ObjectStore, jobs <-chan fileRestoreJob, errs chan<- error) {
	defer wg.Done()
	for job := range jobs {
		// 1. Read the file manifest object.
		manifestBuffer, err := store.ReadObjectAsBuffer(job.ManifestHash)
		if err != nil {
			errs <- fmt.Errorf("failed to read manifest %s for %s: %w", job.ManifestHash, job.DestinationPath, err)
			continue
		}
		var manifest types.FileManifest
		if err := json.Unmarshal(manifestBuffer, &manifest); err != nil {
			errs <- fmt.Errorf("failed to parse manifest %s for %s: %w", job.ManifestHash, job.DestinationPath, err)
			continue
		}

		// 2. Read all data chunks for the file.
		var fileContent []byte
		for _, chunkRef := range manifest.Chunks {
			chunkData, err := store.ReadObjectAsBuffer(chunkRef.Hash)
			if err != nil {
				errs <- fmt.Errorf("failed to read chunk %s for file %s: %w", chunkRef.Hash, job.DestinationPath, err)
				break // Stop processing this file if a chunk is missing
			}
			fileContent = append(fileContent, chunkData...)
		}

		// 3. Write the reconstructed file to disk and set its permissions.
		if err := os.WriteFile(job.DestinationPath, fileContent, job.Mode); err != nil {
			errs <- fmt.Errorf("failed to write file %s: %w", job.DestinationPath, err)
			continue
		}
	}
}

// restoreTree recursively reconstructs a directory from a tree object.
func restoreTree(store *lib.ObjectStore, treeHash, destinationPath string, jobs chan<- fileRestoreJob) error {
	treeBuffer, err := store.ReadObjectAsBuffer(treeHash)
	if err != nil {
		return err
	}
	var tree types.Tree
	if err := json.Unmarshal(treeBuffer, &tree); err != nil {
		return err
	}

	// Ensure the destination directory exists.
	if err := os.MkdirAll(destinationPath, 0755); err != nil {
		return err
	}

	for _, entry := range tree.Entries {
		fullRestorePath := filepath.Join(destinationPath, entry.Name)

		if entry.Type == "blob" {
			// For files, send a job to the worker pool.
			jobs <- fileRestoreJob{
				ManifestHash:    entry.Hash,
				DestinationPath: fullRestorePath,
				Mode:            os.FileMode(entry.Mode),
			}
		} else if entry.Type == "tree" {
			// For directories, recurse synchronously.
			if err := restoreTree(store, entry.Hash, fullRestorePath, jobs); err != nil {
				return err
			}
			// Set permissions on the directory after its contents are processed.
			if err := os.Chmod(fullRestorePath, os.FileMode(entry.Mode)); err != nil {
				// Log a warning, as this is often not a critical failure.
				fmt.Fprintf(os.Stderr, "Warning: could not set mode on directory %s: %v\n", fullRestorePath, err)
			}
		}
	}
	return nil
}

// Restore is the main function for the 'restore' command.
func Restore(sourceDir, snapIdentifier, outputDir string) error {
	absSourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("could not resolve source path: %w", err)
	}
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("could not resolve output path: %w", err)
	}

	store := lib.NewObjectStore(absSourceDir)

	// 1. Find the exact snapshot to restore.
	snapToRestore, err := lib.FindSnap(absSourceDir, snapIdentifier)
	if err != nil {
		return fmt.Errorf("failed to find snapshot %s to restore: %w", snapIdentifier, err)
	}

	// Clean the output directory before restoring.
	// This ensures the restored directory is an exact replica of the snapshot.
	if err := os.RemoveAll(absOutputDir); err != nil {
		return fmt.Errorf("failed to clean output directory: %w", err)
	}
	if err := os.MkdirAll(absOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to recreate output directory: %w", err)
	}

	fmt.Printf("💧 Restoring snap %d (%s) to \"%s\"...\n", snapToRestore.ID, snapToRestore.Hash[:7], absOutputDir)

	// 2. Set up the worker pool.
	jobs := make(chan fileRestoreJob, 100) // Buffered channel
	errs := make(chan error, 100)
	var wg sync.WaitGroup
	numWorkers := runtime.NumCPU()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go restoreFileWorker(&wg, store, jobs, errs)
	}

	// 3. Start the recursive tree traversal.
	// This will populate the jobs channel.
	err = restoreTree(store, snapToRestore.RootTreeHash, absOutputDir, jobs)
	close(jobs) // Signal that no more jobs will be sent.
	if err != nil {
		return fmt.Errorf("failed during tree traversal: %w", err)
	}

	// 4. Wait for all workers to finish.
	wg.Wait()
	close(errs) // Close the errors channel after workers are done.

	// 5. Check if any worker reported an error.
	for restoreErr := range errs {
		if restoreErr != nil {
			// Return the first error we encounter.
			return fmt.Errorf("a restore worker failed: %w", restoreErr)
		}
	}

	fmt.Println("✅ Restore complete!")
	return nil
}
