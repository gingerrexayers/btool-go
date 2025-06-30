package lib

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// setupIgnoreTest creates a temporary directory and writes a .btoolignore file
// with the provided content for isolated testing.
func setupIgnoreTest(t *testing.T, ignoreContent string) string {
	// On macOS, t.TempDir() can return a path that is a symlink (e.g., /var -> /private/var).
	// The function under test, IsPathIgnored, canonicalizes paths by resolving these
	// symlinks. Therefore, the test setup MUST also use the canonical path to ensure
	// that the .btoolignore file is created where the function expects to find it.
	tmpDir := t.TempDir()
	canonicalTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks for temp dir: %v", err)
	}

	ignoreFilePath := filepath.Join(canonicalTmpDir, ".btoolignore")
	if err := os.WriteFile(ignoreFilePath, []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to create .btoolignore file in canonical path: %v", err)
	}

	ResetIgnoreState()
	// Return the canonical path to be used by the test.
	return canonicalTmpDir
}

func TestIsPathIgnored(t *testing.T) {
	// Test case table
	testCases := []struct {
		name            string
		ignoreContent   string
		pathToCheck     string
		shouldBeIgnored bool
	}{
		{
			name:            "Default .git directory ignore",
			ignoreContent:   "", // No user-defined ignores
			pathToCheck:     ".git/config",
			shouldBeIgnored: true,
		},
		{
			name:            "Default .btool directory ignore",
			ignoreContent:   "",
			pathToCheck:     ".btool/objects",
			shouldBeIgnored: true,
		},
		{
			name:            "Default .btoolignore file ignore",
			ignoreContent:   "",
			pathToCheck:     ".btoolignore",
			shouldBeIgnored: true,
		},
		{
			name:            "Specific file match",
			ignoreContent:   "secret.txt",
			pathToCheck:     "secret.txt",
			shouldBeIgnored: true,
		},
		{
			name:            "Glob pattern match (*.log)",
			ignoreContent:   "*.log",
			pathToCheck:     "system.log",
			shouldBeIgnored: true,
		},
		{
			name:            "Glob pattern in subdir",
			ignoreContent:   "*.log",
			pathToCheck:     "logs/system.log",
			shouldBeIgnored: true,
		},
		{
			name:            "Directory pattern match (build/)",
			ignoreContent:   "build/",
			pathToCheck:     "build/asset.js",
			shouldBeIgnored: true,
		},
		{
			name:            "Directory pattern should match the directory itself",
			ignoreContent:   "build/",
			pathToCheck:     "build",
			shouldBeIgnored: true,
		},
		{
			name:            "Negation pattern (!)",
			ignoreContent:   "*.log\n!important.log",
			pathToCheck:     "important.log",
			shouldBeIgnored: false,
		},
		{
			name:            "Negation pattern should not affect other matches",
			ignoreContent:   "*.log\n!important.log",
			pathToCheck:     "unimportant.log",
			shouldBeIgnored: true,
		},
		{
			name:            "Comment and empty lines should be ignored",
			ignoreContent:   "# This is a comment\n\n  \n\n*.tmp",
			pathToCheck:     "some.tmp",
			shouldBeIgnored: true,
		},
		{
			name:            "Path not in ignore list",
			ignoreContent:   "*.log",
			pathToCheck:     "src/main.go",
			shouldBeIgnored: false,
		},
		{
			name:            "Path with Windows-style separators in pattern",
			ignoreContent:   "dist\\main.js",
			pathToCheck:     "dist/main.js",
			shouldBeIgnored: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			testDir := setupIgnoreTest(t, tc.ignoreContent)
			fullPath := filepath.Join(testDir, filepath.FromSlash(tc.pathToCheck))

			// Create the file/dir structure for the path we are testing against.
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				t.Fatalf("Failed to create parent directory for test: %v", err)
			}
			// This creates the final path component. If it's meant to be a directory,
			// this creates a file with that name, which is sufficient for testing.
			if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Act
			isIgnored := IsPathIgnored(testDir, fullPath)

			// Assert
			if isIgnored != tc.shouldBeIgnored {
				t.Errorf("Path '%s' with ignore content:\n---\n%s\n---\nExpected ignored=%v, but got %v",
					tc.pathToCheck, tc.ignoreContent, tc.shouldBeIgnored, isIgnored)
			}
		})
	}
}

func TestIgnoreCaching(t *testing.T) {
	// This test will spy on os.ReadFile to see how many times it's called.
	// Since we can't easily spy on stdlib functions in Go, we will check a side-effect:
	// if we delete the .btoolignore file after the first call, subsequent calls should
	// still use the cached rules and produce the same result.

	// Arrange
	ignoreContent := "cache-test.txt"
	testDir := setupIgnoreTest(t, ignoreContent)

	// Create the file to be tested.
	pathToTest := filepath.Join(testDir, "cache-test.txt")
	if err := os.WriteFile(pathToTest, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	ignoreFilePath := filepath.Join(testDir, BtoolIgnoreFilename)

	// Act (1st call) - This should load and cache the ignore rules.
	isIgnoredFirstCall := IsPathIgnored(testDir, pathToTest)

	// Assert (1st call)
	if !isIgnoredFirstCall {
		t.Fatal("First call failed: path that should be ignored was not.")
	}

	// Arrange for 2nd call: Delete the source of the rules.
	err := os.Remove(ignoreFilePath)
	if err != nil {
		t.Fatalf("Failed to remove .btoolignore for caching test: %v", err)
	}

	// Act (2nd call) - This should hit the cache and NOT re-read the (now deleted) file.
	isIgnoredSecondCall := IsPathIgnored(testDir, pathToTest)

	// Assert (2nd call)
	if !isIgnoredSecondCall {
		t.Error("Second call failed: path was not ignored, indicating cache was not used.")
	}
}

func TestIgnoreConcurrency(t *testing.T) {
	t.Parallel()
	// This test ensures that the caching mechanism is thread-safe.
	// We'll have many goroutines all trying to access the ignore rules for the
	// same directory at the same time.

	// Arrange
	testDir := setupIgnoreTest(t, "*.log")

	// Create the files to be tested by the goroutines. This is critical because
	// filepath.EvalSymlinks needs the files to exist to work correctly.
	logFilePath := filepath.Join(testDir, "test.log")
	txtFilePath := filepath.Join(testDir, "test.txt")
	if err := os.WriteFile(logFilePath, []byte("log"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(txtFilePath, []byte("txt"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Act
	numGoroutines := 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// Concurrently check both an ignored and a non-ignored file.
			if !IsPathIgnored(testDir, logFilePath) {
				t.Error("Concurrent check failed: .log file should have been ignored")
			}
			if IsPathIgnored(testDir, txtFilePath) {
				t.Error("Concurrent check failed: .txt file should not have been ignored")
			}
		}()
	}

	wg.Wait()
}
