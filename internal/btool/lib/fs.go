package lib

import (
	"io"
	"os"
)

// CopyFile copies a file from src to dst. If dst does not exist, it is created.
// If it does exist, it is overwritten.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Ensure the data is written to stable storage.
	return destFile.Sync()
}
