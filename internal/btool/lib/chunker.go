// Package lib contains the core, reusable services for the btool application.
package lib

import (
	"bytes"
	"io"
	"os"

	"github.com/aclements/go-rabin/rabin"
	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// Constants for the Rabin chunker configuration.
const (
	// These values determine the target chunk sizes.
	minChunkSize = 4 * 1024  // 4KB
	avgChunkSize = 8 * 1024  // 8KB
	maxChunkSize = 16 * 1024 // 16KB

	// A 64-bit irreducible polynomial over GF(2).
	defaultPoly = rabin.Poly64
	// The size of the rolling hash window.
	defaultWindowSize = 64
)

// rabinTable is a pre-computed table for the Rabin chunker.
// Initializing this is computationally expensive, so we do it once and reuse it.
var rabinTable = rabin.NewTable(defaultPoly, defaultWindowSize)

// ChunkFile reads a file from disk, splits it into variable-sized chunks using
// Rabin fingerprinting, and returns a slice of Chunk objects containing the
// data and hash of each chunk, along with the total file size.
func ChunkFile(filePath string) ([]types.Chunk, int64, error) {
	// 1. Read the entire file into memory. For very large files, a streaming
	// implementation would be more memory-efficient.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0, err
	}

	// If the file is empty, there's nothing to chunk.
	if len(content) == 0 {
		return []types.Chunk{}, 0, nil
	}

	// 2. Create a reader from the in-memory content.
	reader := bytes.NewReader(content)

	// 3. Create a new Rabin chunker using our pre-computed table and chunk size settings.
	chunker := rabin.NewChunker(rabinTable, reader, minChunkSize, avgChunkSize, maxChunkSize)

	var chunks []types.Chunk
	var totalSize int64
	var offset int64

	// 4. Loop, calling Next() to get the length of each chunk.
	for {
		length, err := chunker.Next()
		if err == io.EOF {
			// We've reached the end of the file.
			break
		}
		if err != nil {
			return nil, 0, err
		}

		// 5. Use the length to slice the original content buffer. This is efficient
		// as it avoids copying the data for each chunk.
		chunkData := content[offset : offset+int64(length)]
		offset += int64(length)

		// 6. Create the Chunk object with its data and hash.
		hash := GetHash(chunkData) // Assumes GetHash() from hasher.go
		size := int64(len(chunkData))
		totalSize += size

		chunks = append(chunks, types.Chunk{
			Hash: hash,
			Size: size,
			Data: chunkData,
		})
	}

	// 7. Handle the edge case where a file is smaller than the minimum chunk size.
	// In this case, the chunker may not produce any chunks, so we treat the
	// entire file as a single chunk.
	if len(chunks) == 0 && len(content) > 0 {
		hash := GetHash(content)
		size := int64(len(content))
		chunks = append(chunks, types.Chunk{Hash: hash, Size: size, Data: content})
		totalSize = size
	}

	return chunks, totalSize, nil
}
