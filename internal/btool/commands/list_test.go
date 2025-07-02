package commands_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdout is a helper function to redirect os.Stdout to an in-memory
// buffer, execute a function, and then return the captured output.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// This channel will signal when the output has been fully read.
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	// Execute the function that will print to stdout.
	f()

	// Clean up: close the writer and restore the original stdout.
	_ = w.Close()
	os.Stdout = oldStdout

	// Read the output from the channel and return.
	return <-outC
}

func TestListCommand(t *testing.T) {
	t.Run("should correctly list snapshots and show snap size", func(t *testing.T) {
		// Arrange: Create a test repository with two snapshots.
		testDir := t.TempDir()
		file1Path := filepath.Join(testDir, "file1.txt")

		err := os.WriteFile(file1Path, []byte("version 1"), 0644)
		require.NoError(t, err, "Setup failed: could not write file1")

		err = commands.Snap(testDir, "first commit")
		require.NoError(t, err, "Setup failed: first snap command failed")

		time.Sleep(10 * time.Millisecond)

		// Modify the file to ensure the second snapshot has a non-zero snap size.
		err = os.WriteFile(file1Path, []byte("version 2 is a bit longer"), 0644)
		require.NoError(t, err, "Setup failed: could not modify file1")

		err = commands.Snap(testDir, "second commit")
		require.NoError(t, err, "Setup failed: second snap command failed")

		// Act: Capture the output of the List command.
		var listErr error
		output := captureStdout(t, func() {
			listErr = commands.List(testDir)
		})
		require.NoError(t, listErr, "commands.List() returned an unexpected error")

		// Assert
		// General output checks
		assert.Contains(t, output, "Snaps for", "Output is missing the header line")
		assert.Contains(t, output, "1         ", "Output is missing the entry for snapshot ID 1")
		assert.Contains(t, output, "2         ", "Output is missing the entry for snapshot ID 2")

		// Specific checks for Snap Size column
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var headerLine string
		for _, line := range lines {
			if strings.Contains(line, "SNAPSHOT") && strings.Contains(line, "HASH") {
				headerLine = line
				break
			}
		}
		require.NotEmpty(t, headerLine, "Could not find header line in output")
		assert.Contains(t, headerLine, "SNAP SIZE", "Header is missing the 'SNAP SIZE' column")

		snapSizeCol := strings.Index(headerLine, "SNAP SIZE")

		var snap2Line string
		for _, line := range lines {
			if strings.HasPrefix(line, "2         ") {
				snap2Line = line
				break
			}
		}
		require.NotEmpty(t, snap2Line, "Could not find line for snapshot 2 in output")

		require.GreaterOrEqual(t, len(snap2Line), snapSizeCol+15, "Line for snapshot 2 is too short to contain snap size: %s", snap2Line)

		snapSizeVal := strings.TrimSpace(snap2Line[snapSizeCol : snapSizeCol+15])
		assert.NotEqual(t, "0 Bytes", snapSizeVal, "Expected a non-zero snap size for snapshot 2")
		assert.NotEmpty(t, snapSizeVal, "Expected a non-zero snap size for snapshot 2")
	})

	t.Run("should show a message when no snaps exist", func(t *testing.T) {
		// Arrange
		testDir := t.TempDir()

		// Act
		var listErr error
		output := captureStdout(t, func() {
			listErr = commands.List(testDir)
		})

		// Assert
		require.NoError(t, listErr, "commands.List() returned an unexpected error")
		assert.Contains(t, output, "No snaps found", "Expected 'No snaps found' message")
	})

	t.Run("should return an error for a non-existent directory", func(t *testing.T) {
		// Arrange
		nonExistentDir := filepath.Join(t.TempDir(), "this_does_not_exist")

		// Act
		err := commands.List(nonExistentDir)

		// Assert
		require.Error(t, err, "Expected an error for a non-existent directory, but got nil")
		assert.Contains(t, err.Error(), "target directory does not exist")
	})
}
