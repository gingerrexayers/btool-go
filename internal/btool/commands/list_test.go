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
)

// captureStdout is a helper function to redirect os.Stdout to an in-memory
// buffer, execute a function, and then return the captured output.
func captureStdout(f func()) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
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
	return <-outC, nil
}

func TestListCommand(t *testing.T) {
	t.Run("should correctly list multiple snapshots", func(t *testing.T) {
		// Arrange: Create a test repository with two snapshots.
		testDir := t.TempDir()
		err := os.WriteFile(filepath.Join(testDir, "file1.txt"), []byte("version 1"), 0644)
		if err != nil {
			t.Fatalf("Setup failed: could not write file1: %v", err)
		}
		// Run the real Snap command to create the first snapshot.
		err = commands.Snap(testDir, "first commit")
		if err != nil {
			t.Fatalf("Setup failed: first snap command failed: %v", err)
		}

		// A small delay to ensure a different timestamp for the next snapshot.
		time.Sleep(10 * time.Millisecond)

		err = os.WriteFile(filepath.Join(testDir, "file2.txt"), []byte("version 2"), 0644)
		if err != nil {
			t.Fatalf("Setup failed: could not write file2: %v", err)
		}
		// Run the real Snap command again.
		err = commands.Snap(testDir, "second commit")
		if err != nil {
			t.Fatalf("Setup failed: second snap command failed: %v", err)
		}

		// Act: Capture the output of the List command.
		var listErr error
		output, err := captureStdout(func() {
			listErr = commands.List(testDir)
		})
		if err != nil {
			t.Fatalf("Failed to capture stdout: %v", err)
		}

		// Assert
		if listErr != nil {
			t.Fatalf("commands.List() returned an unexpected error: %v", listErr)
		}

		// Check the output for key components.
		if !strings.Contains(output, "Snaps for") {
			t.Error("Output is missing the header line")
		}
		if !strings.Contains(output, "SNAPSHOT") || !strings.Contains(output, "TIMESTAMP") {
			t.Error("Output is missing the table headers")
		}
		if !strings.Contains(output, "1         ") {
			t.Error("Output is missing the entry for snapshot ID 1")
		}
		if !strings.Contains(output, "2         ") {
			t.Error("Output is missing the entry for snapshot ID 2")
		}
		if !strings.Contains(output, "Total stored size") {
			t.Error("Output is missing the total stored size summary line")
		}

		// Check that there are the correct number of lines (header, title, separator, 2 snaps, 2 blanks, total)
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) != 7 {
			t.Errorf("Expected 7 lines of output, but got %d. Output:\n%s", len(lines), output)
		}
	})

	t.Run("should show a message when no snaps exist", func(t *testing.T) {
		// Arrange
		testDir := t.TempDir()

		// Act
		var listErr error
		output, err := captureStdout(func() {
			listErr = commands.List(testDir)
		})
		if err != nil {
			t.Fatalf("Failed to capture stdout: %v", err)
		}

		// Assert
		if listErr != nil {
			t.Fatalf("commands.List() returned an unexpected error: %v", listErr)
		}
		if !strings.Contains(output, "No snaps found") {
			t.Errorf("Expected 'No snaps found' message, but got: %s", output)
		}
	})

	t.Run("should return an error for a non-existent directory", func(t *testing.T) {
		// Arrange
		nonExistentDir := filepath.Join(t.TempDir(), "this_does_not_exist")

		// Act
		err := commands.List(nonExistentDir)

		// Assert
		if err == nil {
			t.Fatal("Expected an error for a non-existent directory, but got nil")
		}
		if !strings.Contains(err.Error(), "target directory does not exist") {
			t.Errorf("Expected error to mention 'target directory does not exist', but got: %v", err)
		}
	})
}
