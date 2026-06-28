package compactor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTask_Validate verifies the structural invariants and constraints checks of Task.Validate().
func TestTask_Validate(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "temp.txt")
	if err := os.WriteFile(tempFile, []byte("test"), 0x0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	tests := []struct {
		name        string
		task        *Task
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil task",
			task:        nil,
			wantErr:     true,
			errContains: "compaction task is nil",
		},
		{
			name: "no input files",
			task: &Task{
				InputFiles:      []string{},
				FileIDs:         []int{},
				OutputDirectory: tempDir,
			},
			wantErr:     true,
			errContains: "requires at least one input file",
		},
		{
			name: "length mismatch",
			task: &Task{
				InputFiles:      []string{"a.sst"},
				FileIDs:         []int{1, 2},
				OutputDirectory: tempDir,
			},
			wantErr:     true,
			errContains: "does not match FileIDs length",
		},
		{
			name: "output directory does not exist",
			task: &Task{
				InputFiles:      []string{"a.sst"},
				FileIDs:         []int{1},
				OutputDirectory: filepath.Join(tempDir, "non_existent_dir"),
			},
			wantErr:     true,
			errContains: "output directory does not exist",
		},
		{
			name: "output directory is not a directory",
			task: &Task{
				InputFiles:      []string{"a.sst"},
				FileIDs:         []int{1},
				OutputDirectory: tempFile,
			},
			wantErr:     true,
			errContains: "output directory path is not a directory",
		},
		{
			name: "duplicate FileID",
			task: &Task{
				InputFiles:      []string{"a.sst", "b.sst"},
				FileIDs:         []int{1, 1},
				OutputDirectory: tempDir,
			},
			wantErr:     true,
			errContains: "duplicate FileID found",
		},
		{
			name: "valid task",
			task: &Task{
				InputFiles:      []string{"a.sst", "b.sst"},
				FileIDs:         []int{1, 2},
				OutputDirectory: tempDir,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Validate() error = %v, want string containing %q", err, tt.errContains)
			}
		})
	}
}
