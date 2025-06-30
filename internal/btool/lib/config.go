// Package lib contains the core, reusable services for the btool application.
package lib

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/denormal/go-gitignore"
)

// --- Constants ---

// BtoolDirName is the name of the root directory for all backup data.
const BtoolDirName = ".btool"

// ObjectsDirName is the name of the subdirectory for content-addressed objects.
const ObjectsDirName = "objects"

// SnapsDirName is the name of the subdirectory for snapshot manifest files.
const SnapsDirName = "snaps"

// PacksDirName is the name of the subdirectory for packed object files.
const PacksDirName = "packs"

// BtoolIgnoreFilename is the name of the file containing user-defined ignore patterns.
const BtoolIgnoreFilename = ".btoolignore"

// HashAlgorithm is the chosen hashing algorithm. Using a constant here allows
// for easy swapping/testing and ensures consistency across the app.
const HashAlgorithm = "sha256"

// --- Package-level Variables ---

// defaultIgnorePatterns contains the essential directories that should always be ignored.
var defaultIgnorePatterns = []string{
	// Use glob patterns for directories to ensure they work with the gitignore library
	".git/**",
	BtoolDirName + "/**",
	// Files should not have trailing slash.
	BtoolIgnoreFilename,
}

var (
	// ignoreCache stores compiled gitignore.GitIgnore objects to avoid re-reading
	// and re-parsing the .btoolignore file. The key is the canonical absolute
	// path to a directory. Access to this cache is serialized by a global mutex
	// to ensure thread safety.
	ignoreCache = make(map[string]gitignore.GitIgnore)
	cacheMutex  = &sync.Mutex{}
)

// --- Path Helper Functions ---
// These functions use path/filepath for OS-agnostic path construction.

// GetBtoolDir returns the absolute path to the .btool directory for a given base directory.
func GetBtoolDir(baseDir string) string {
	return filepath.Join(baseDir, BtoolDirName)
}

// GetObjectsDir returns the absolute path to the objects subdirectory.
func GetObjectsDir(baseDir string) string {
	return filepath.Join(GetBtoolDir(baseDir), ObjectsDirName)
}

// GetSnapsDir returns the absolute path to the snaps subdirectory.
func GetSnapsDir(baseDir string) string {
	return filepath.Join(GetBtoolDir(baseDir), SnapsDirName)
}

// GetPacksDir returns the absolute path to the packs subdirectory.
func GetPacksDir(baseDir string) string {
	return filepath.Join(GetBtoolDir(baseDir), PacksDirName)
}

// GetIndexPath returns the absolute path to the index.json file.
func GetIndexPath(baseDir string) string {
	return filepath.Join(GetBtoolDir(baseDir), "index.json")
}

// BtoolPaths holds the structured paths for the btool directory.
type BtoolPaths struct {
	BtoolDir   string
	ObjectsDir string
	SnapsDir   string
	PacksDir   string
}

// EnsureBtoolDirs ensures that the core .btool directories exist, creating them
// if necessary. It is idempotent.
// It returns a struct containing the created paths and an error if creation fails.
func EnsureBtoolDirs(baseDir string) (BtoolPaths, error) {
	paths := BtoolPaths{
		BtoolDir:   GetBtoolDir(baseDir),
		ObjectsDir: GetObjectsDir(baseDir),
		SnapsDir:   GetSnapsDir(baseDir),
		PacksDir:   GetPacksDir(baseDir),
	}

	// os.MkdirAll is the equivalent of `mkdir -p`. It creates all necessary parent
	// directories and does not return an error if the path already exists.
	// We create the most nested paths first.
	if err := os.MkdirAll(paths.ObjectsDir, 0755); err != nil {
		return BtoolPaths{}, err
	}
	if err := os.MkdirAll(paths.SnapsDir, 0755); err != nil {
		return BtoolPaths{}, err
	}
	if err := os.MkdirAll(paths.PacksDir, 0755); err != nil {
		return BtoolPaths{}, err
	}

	return paths, nil
}

// IsPathIgnored checks if a given path relative to the baseDir should be ignored.
// It uses a cache to avoid recompiling ignore rules for the same directory.
func IsPathIgnored(baseDir, path string) bool {
	// Lock the mutex for the entire duration of the function to serialize all
	// access. This is a "brute-force" thread-safety measure taken because the
	// gitignore library appears to have issues with concurrent use, even when
	// creating new matchers.
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// We MUST use the same canonical pathing for both arguments to filepath.Rel.
	// First, get the canonical version of the base directory.
	canonicalBaseDir, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		canonicalBaseDir = baseDir // Fallback on error.
	}

	// Get the ignore matcher from the cache, or load it if it's not present.
	matcher, found := ignoreCache[canonicalBaseDir]
	if !found {
		matcher = loadIgnoreMatcher(canonicalBaseDir)
		ignoreCache[canonicalBaseDir] = matcher
	}

	// Second, get the canonical version of the path we are checking.
	canonicalPathToCheck, err := filepath.EvalSymlinks(path)
	if err != nil {
		canonicalPathToCheck = path // Fallback on error.
	}

	// Now that both paths are canonical, we can safely find the relative path.
	relativePath, err := filepath.Rel(canonicalBaseDir, canonicalPathToCheck)
	if err != nil {
		// If we can't determine the relative path, it's safest not to ignore.
		return false
	}
	// The gitignore library expects forward-slash separators, even on Windows.
	slashedPath := filepath.ToSlash(relativePath)

	// Try matching with relative path first
	match := matcher.Match(slashedPath)
	if match == nil {
		// If relative path doesn't work, try absolute path
		match = matcher.Match(canonicalPathToCheck)
	}
	if match == nil {
		return false
	}
	return match.Ignore()
}

// loadIgnoreMatcher loads ignore patterns and compiles them into a gitignore.GitIgnore object.
func loadIgnoreMatcher(baseDir string) gitignore.GitIgnore {
	// 1. Start with the default patterns.
	rawPatterns := make([]string, len(defaultIgnorePatterns))
	copy(rawPatterns, defaultIgnorePatterns)

	// 2. Read patterns from the .btoolignore file, if it exists.
	ignoreFilePath := filepath.Join(baseDir, ".btoolignore")
	if _, err := os.Stat(ignoreFilePath); err == nil {
		content, err := os.ReadFile(ignoreFilePath)
		if err == nil {
			// Split the content into lines and add to the raw patterns.
			lines := strings.Split(string(content), "\n")
			rawPatterns = append(rawPatterns, lines...)
		}
	}

	// 3. Clean up the patterns: remove comments and trim whitespace.
	var finalPatterns []string
	for _, p := range rawPatterns {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			// Normalize Windows-style backslashes to forward slashes for cross-platform compatibility
			trimmed = strings.ReplaceAll(trimmed, "\\", "/")
			
			// Convert directory patterns (ending with /) to glob patterns for better gitignore compatibility
			if strings.HasSuffix(trimmed, "/") && !strings.HasSuffix(trimmed, "**/") {
				trimmed = trimmed + "**"
			}
			finalPatterns = append(finalPatterns, trimmed)
		}
	}

	// 4. Compile the patterns into a matcher.
	combinedPatterns := strings.Join(finalPatterns, "\n")
	reader := strings.NewReader(combinedPatterns)
	matcher := gitignore.New(
		reader,
		baseDir,
		// The error handler tells the parser to continue on error.
		func(err gitignore.Error) bool { return false },
	)

	// If the matcher fails to compile, return a "null" matcher that ignores nothing.
	if matcher == nil {
		return gitignore.New(strings.NewReader(""), "", nil)
	}

	return matcher
}

// ResetIgnoreState clears the ignore cache. This is used for testing.
func ResetIgnoreState() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	ignoreCache = make(map[string]gitignore.GitIgnore)
}
