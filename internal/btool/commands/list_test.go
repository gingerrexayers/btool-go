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
	t.Run("should correctly list snapshots and show snap size", func(t *testing.T) {
		// Arrange: Create a test repository with two snapshots.
		testDir := t.TempDir()
		file1Path := filepath.Join(testDir, "file1.txt")

		err := os.WriteFile(file1Path, []byte("version 1"), 0644)
		if err != nil {
			t.Fatalf("Setup failed: could not write file1: %v", err)
		}
		err = commands.Snap(testDir, "first commit")
		if err != nil {
			t.Fatalf("Setup failed: first snap command failed: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		// Modify the file to ensure the second snapshot has a non-zero snap size.
		err = os.WriteFile(file1Path, []byte("version 2 is a bit longer"), 0644)
		if err != nil {
			t.Fatalf("Setup failed: could not modify file1: %v", err)
		}
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

		// General output checks
		if !strings.Contains(output, "Snaps for") {
			t.Error("Output is missing the header line")
		}
		if !strings.Contains(output, "1         ") {
			t.Error("Output is missing the entry for snapshot ID 1")
		}
		if !strings.Contains(output, "2         ") {
			t.Error("Output is missing the entry for snapshot ID 2")
		}

		// Specific checks for Snap Size column
		lines := strings.Split(strings.TrimSpace(output), "\n")
		headerLine := ""
		for _, line := range lines {
			if strings.Contains(line, "SNAPSHOT") && strings.Contains(line, "HASH") {
				headerLine = line
				break
			}
		}
		if headerLine == "" {
			t.Fatal("Could not find header line in output")
		}

		if !strings.Contains(headerLine, "SNAP SIZE") {
			t.Error("Header is missing the 'SNAP SIZE' column")
		}

		snapSizeCol := strings.Index(headerLine, "SNAP SIZE")

		snap2Line := ""
		for _, line := range lines {
			if strings.HasPrefix(line, "2         ") {
				snap2Line = line
				break
			}
		}
		if snap2Line == "" {
			t.Fatal("Could not find line for snapshot 2 in output")
		}

		if len(snap2Line) < snapSizeCol+15 {
			t.Fatalf("Line for snapshot 2 is too short to contain snap size: %s", snap2Line)
		}
		snapSizeVal := strings.TrimSpace(snap2Line[snapSizeCol : snapSizeCol+15])
		if snapSizeVal == "0 Bytes" || snapSizeVal == "" {
			t.Errorf("Expected a non-zero snap size for snapshot 2, but got '%s'", snapSizeVal)
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
