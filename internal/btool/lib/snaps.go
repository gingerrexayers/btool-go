package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gingerrexayers/btool-go/internal/btool/types"
)

// SnapDetail enhances the Snap struct with the calculated ID and hash (filename).
type SnapDetail struct {
	ID           int
	Hash         string
	Timestamp    time.Time
	Message      string
	RootTreeHash string
	SourceSize   int64
}

// GetSortedSnaps reads all snaps for a given repository, sorts them by date
// (oldest first), and returns them with a sequential ID.
func GetSortedSnaps(baseDir string) ([]SnapDetail, error) {
	snapsDir := GetSnapsDir(baseDir)

	dirEntries, err := os.ReadDir(snapsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapDetail{}, nil // No snaps dir exists, so no snaps. Not an error.
		}
		return nil, err // A different error occurred (e.g., permissions).
	}

	var snapDetails []SnapDetail
	for _, entry := range dirEntries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			snapHash := entry.Name()[:len(entry.Name())-len(".json")]

			content, err := os.ReadFile(filepath.Join(snapsDir, entry.Name()))
			if err != nil {
				// Log a warning but continue, in case one snap file is corrupted.
				// fmt.Fprintf(os.Stderr, "Warning: could not read snap file %s: %v\n", entry.Name(), err)
				continue
			}

			var snapData types.Snap
			if err := json.Unmarshal(content, &snapData); err != nil {
				// fmt.Fprintf(os.Stderr, "Warning: could not parse snap file %s: %v\n", entry.Name(), err)
				continue
			}

			ts, err := time.Parse(time.RFC3339, snapData.Timestamp)
			if err != nil {
				// fmt.Fprintf(os.Stderr, "Warning: could not parse timestamp in snap file %s: %v\n", entry.Name(), err)
				continue
			}

			snapDetails = append(snapDetails, SnapDetail{
				Hash:         snapHash,
				Timestamp:    ts,
				Message:      snapData.Message,
				RootTreeHash: snapData.RootTreeHash,
				SourceSize:   snapData.SourceSize,
			})
		}
	}

	// Sort by timestamp, oldest first.
	sort.Slice(snapDetails, func(i, j int) bool {
		return snapDetails[i].Timestamp.Before(snapDetails[j].Timestamp)
	})

	// Assign sequential IDs based on the sorted order.
	for i := range snapDetails {
		snapDetails[i].ID = i + 1
	}

	return snapDetails, nil
}

// FindSnap searches for a snapshot by a given identifier, which can be a numeric ID or a hash prefix.
func FindSnap(baseDir, snapIdentifier string) (*SnapDetail, error) {
	snaps, err := GetSortedSnaps(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshots: %w", err)
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snaps found to search from")
	}

	var snapToReturn *SnapDetail
	snapID, err := strconv.Atoi(snapIdentifier)
	if err == nil { // Identifier is a numeric ID.
		for i := range snaps {
			if snaps[i].ID == snapID {
				snapToReturn = &snaps[i]
				break
			}
		}
	} else { // Identifier is a hash prefix.
		var matches []*SnapDetail
		for i := range snaps {
			if strings.HasPrefix(snaps[i].Hash, snapIdentifier) {
				matches = append(matches, &snaps[i])
			}
		}
		if len(matches) == 1 {
			snapToReturn = matches[0]
		} else if len(matches) > 1 {
			return nil, fmt.Errorf("ambiguous snap identifier '%s' matches multiple snapshots", snapIdentifier)
		}
	}

	if snapToReturn == nil {
		return nil, fmt.Errorf("no snap found with ID or hash prefix '%s'", snapIdentifier)
	}

	return snapToReturn, nil
}
