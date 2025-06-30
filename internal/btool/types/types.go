package types

// `json:"..."` tags are used for serialization/deserialization, which is very useful.

type ChunkRef struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Chunk represents a piece of a file's data. The Data field is not serialized.
type Chunk struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Data []byte `json:"-"`
}

type FileManifest struct {
	Chunks    []ChunkRef `json:"chunks"`
	TotalSize int64      `json:"totalSize"`
}

type TreeEntry struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	Type string `json:"type"` // "blob" or "tree"
	Mode uint32 `json:"mode"`
}

type Tree struct {
	Entries []TreeEntry `json:"entries"`
}

type Snap struct {
	Timestamp    string `json:"timestamp"`
	RootTreeHash string `json:"rootTreeHash"`
	Message      string `json:"message,omitempty"`
	SourceSize   int64  `json:"sourceSize"`
}

type PackIndexEntry struct {
	PackHash string `json:"packHash"`
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
}

type PackIndex map[string]PackIndexEntry
