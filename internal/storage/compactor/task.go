package compactor

import (
	"fmt"
	"os"
)

// Task defines the input files, file metadata identifiers, output destination,
// segment configuration, and level traits for a single compaction run.
type Task struct {
	InputFiles      []string
	FileIDs         []int
	OutputDirectory string
	NextSegmentID   int
	IsBottomLevel   bool
}

// Result describes the stats of a completed compaction run, including files created,
// obsolete files to clean up, and performance metrics (bytes and keys written).
type Result struct {
	NewFilesCreated []string
	ObsoleteFiles   []string
	BytesWritten    uint64
	KeysWritten     uint32
}

// Validate checks the task configuration parameters for structural invariants and constraints.
// It verifies that there is at least one input file, slice lengths match, the output directory
// is accessible, and that all FileIDs are unique.
func (task *Task) Validate() error {
	if task == nil {
		return fmt.Errorf("compaction task is nil")
	}
	if len(task.InputFiles) == 0 {
		return fmt.Errorf("compaction task requires at least one input file")
	}
	if len(task.InputFiles) != len(task.FileIDs) {
		return fmt.Errorf("InputFiles length (%d) does not match FileIDs length (%d)",
			len(task.InputFiles), len(task.FileIDs))
	}

	info, err := os.Stat(task.OutputDirectory)
	if err != nil {
		return fmt.Errorf("output directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output directory path is not a directory: %s", task.OutputDirectory)
	}

	seen := make(map[int]struct{}, len(task.FileIDs))
	for _, id := range task.FileIDs {
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate FileID found: %d", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
