// Package commands contains the command-line interface for the btool application.
package commands

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/lib"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// fileProcessResult is a struct to hold the outcome of processing a single file in a worker.
type fileProcessResult struct {
	FilePath     string
	ManifestHash string
	TotalSize    int64
	Err          error
}

// findAllFiles walks the directory tree and returns a slice of all file paths
// to be included in the snapshot, respecting the .btoolignore configuration.
func findAllFiles(rootDir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == rootDir {
			return nil
		}

		if lib.IsPathIgnored(rootDir, path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}

// processFilesConcurrently creates a worker pool of goroutines to process files in parallel.
// It chunks, hashes, and writes all file data (chunks and manifests) to the object store.
func processFilesConcurrently(baseDir string, files []string) (map[string]string, int64, error) {
	numJobs := len(files)
	jobs := make(chan string, numJobs)
	results := make(chan fileProcessResult, numJobs)

	// Use a WaitGroup to wait for all goroutines to finish.
	var wg sync.WaitGroup
	numWorkers := runtime.NumCPU()

	// Start worker goroutines.
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				// --- This is the work each goroutine does ---
				chunks, totalSize, err := lib.ChunkFile(filePath)
				if err != nil {
					results <- fileProcessResult{FilePath: filePath, Err: err}
					continue
				}

				// Write all data chunks to the pending object store.
				for _, chunk := range chunks {
					if _, err := lib.WriteObject(baseDir, chunk.Data); err != nil {
						results <- fileProcessResult{FilePath: filePath, Err: err}
						return // Use return to stop processing on this file
					}
				}

				// Create and write the file manifest object.
				chunkRefs := make([]types.ChunkRef, len(chunks))
				for i, c := range chunks {
					chunkRefs[i] = types.ChunkRef{Hash: c.Hash, Size: c.Size}
				}
				manifest := types.FileManifest{Chunks: chunkRefs, TotalSize: totalSize}
				manifestJSON, _ := json.Marshal(manifest)
				manifestHash, err := lib.WriteObject(baseDir, manifestJSON)
				if err != nil {
					results <- fileProcessResult{FilePath: filePath, Err: err}
					continue
				}

				results <- fileProcessResult{FilePath: filePath, ManifestHash: manifestHash, TotalSize: totalSize}
			}
		}()
	}

	// Send all file paths to the jobs channel.
	for _, file := range files {
		jobs <- file
	}
	close(jobs) // Signal that no more jobs will be sent.

	// Wait for all workers to finish, then close the results channel.
	wg.Wait()
	close(results)

	// Collect results and check for errors.
	fileHashes := make(map[string]string)
	var totalSourceSize int64
	for res := range results {
		if res.Err != nil {
			return nil, 0, fmt.Errorf("failed to process file %s: %w", res.FilePath, res.Err)
		}
		fileHashes[res.FilePath] = res.ManifestHash
		totalSourceSize += res.TotalSize
	}

	return fileHashes, totalSourceSize, nil
}

// buildTree recursively traverses a directory path and constructs a Tree object,
// saving it to the object store and returning its hash.
func buildTree(baseDir, directoryPath string, fileHashes map[string]string) (string, error) {
	entries := []types.TreeEntry{}
	dirEntries, err := os.ReadDir(directoryPath)
	if err != nil {
		return "", err
	}

	for _, entry := range dirEntries {
		fullPath := filepath.Join(directoryPath, entry.Name())
		if lib.IsPathIgnored(baseDir, fullPath) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return "", err
		}

		if entry.IsDir() {
			treeHash, err := buildTree(baseDir, fullPath, fileHashes)
			if err != nil {
				return "", err
			}
			entries = append(entries, types.TreeEntry{
				Name: entry.Name(),
				Hash: treeHash,
				Type: "tree",
				Mode: uint32(info.Mode().Perm()),
			})
		} else {
			manifestHash, ok := fileHashes[fullPath]
			if !ok {
				return "", fmt.Errorf("missing manifest hash for file: %s", fullPath)
			}
			entries = append(entries, types.TreeEntry{
				Name: entry.Name(),
				Hash: manifestHash,
				Type: "blob",
				Mode: uint32(info.Mode().Perm()),
			})
		}
	}

	// Sort entries for deterministic tree hashing.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	tree := types.Tree{Entries: entries}
	treeJSON, _ := json.Marshal(tree)
	treeHash, err := lib.WriteObject(baseDir, treeJSON)
	if err != nil {
		return "", err
	}
	return treeHash, nil
}

// Snap is the main function for the 'snap' command. It orchestrates the entire
// snapshotting process.
func Snap(targetDirectory string, message string) error {
	// 1. Initial setup and validation
	absTargetPath, err := filepath.Abs(targetDirectory)
	if err != nil {
		return fmt.Errorf("could not resolve absolute path for %s: %w", targetDirectory, err)
	}
	if _, err := os.Stat(absTargetPath); os.IsNotExist(err) {
		return fmt.Errorf("target directory does not exist: %s", absTargetPath)
	}

	fmt.Printf("📷 Starting snap for \"%s\"...\n", absTargetPath)
	lib.ResetObjectStoreState() // Ensure a clean state for this command run.
	if _, err := lib.EnsureBtoolDirs(absTargetPath); err != nil {
		return fmt.Errorf("failed to ensure .btool directories: %w", err)
	}

	// 2. Find all files to be processed.
	files, err := findAllFiles(absTargetPath)
	if err != nil {
		return fmt.Errorf("error finding files: %w", err)
	}

	fmt.Printf("   - Found %d files to process...\n", len(files))

	// 3. Process files concurrently to generate chunks and manifests.
	fileHashes, totalSourceSize, err := processFilesConcurrently(absTargetPath, files)
	if err != nil {
		return fmt.Errorf("error processing files: %w", err)
	}
	fmt.Println("   - Finished processing files.")

	// 4. Build the directory tree structure.
	rootTreeHash, err := buildTree(absTargetPath, absTargetPath, fileHashes)
	if err != nil {
		return fmt.Errorf("error building directory tree: %w", err)
	}

	// 5. Create and save the final Snap object.
	snap := types.Snap{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		RootTreeHash: rootTreeHash,
		Message:      message,
		SourceSize:   totalSourceSize,
	}
	snapJSON, _ := json.MarshalIndent(snap, "", "  ")
	snapHash := lib.GetHash(snapJSON)
	snapPath := filepath.Join(lib.GetSnapsDir(absTargetPath), snapHash+".json")
	if err := os.WriteFile(snapPath, snapJSON, 0644); err != nil {
		return fmt.Errorf("failed to write snap manifest: %w", err)
	}

	// 6. Commit all pending objects to a new packfile.
	if err := lib.Commit(absTargetPath); err != nil {
		return fmt.Errorf("failed to commit objects: %w", err)
	}

	fmt.Println("✅ Snap complete!")
	fmt.Printf("   - Snap Hash: %s\n", snapHash)
	fmt.Printf("   - Root Tree Hash: %s\n", rootTreeHash)
	return nil
}
