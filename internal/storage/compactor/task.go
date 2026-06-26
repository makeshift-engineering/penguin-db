package compactor

import "fmt"

type Task struct {
	InputFiles      []string
	FileIDs         []int
	OutputDirectory string
	NextSegmentID   int
	IsBottomLevel   bool
}

type Result struct {
	NewFilesCreated []string
	ObsoleteFiles   []string
	BytesWritten    uint64
	KeysWritten     uint32
}

func (task *Task) Validate() error {
	if len(task.InputFiles) == 0 {
		return fmt.Errorf("compaction task requires at least one input file")
	}
	if len(task.InputFiles) != len(task.FileIDs) {
		return fmt.Errorf("InputFiles length (%d) does not match FileIDs length (%d)",
			len(task.InputFiles), len(task.FileIDs))
	}
	return nil
}
