package compactor

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

func TestCompactor_MergeAndDeduplicate(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 3)
	if err != nil {
		t.Fatalf("failed to create writer 1: %v", err)
	}
	_ = w1.Add([]byte("a"), []byte("v1-old"), sstable.OpcodePut)
	_ = w1.Add([]byte("b"), []byte("v2"), sstable.OpcodePut)
	_ = w1.Add([]byte("c"), []byte("v3-old"), sstable.OpcodePut)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer 1: %v", err)
	}

	path2 := filepath.Join(dir, "input2.sst")
	w2, err := sstable.NewWriter(path2, 3)
	if err != nil {
		t.Fatalf("failed to create writer 2: %v", err)
	}
	_ = w2.Add([]byte("a"), []byte("v1-new"), sstable.OpcodePut)
	_ = w2.Add([]byte("c"), nil, sstable.OpcodeDelete)
	_ = w2.Add([]byte("d"), []byte("v4"), sstable.OpcodePut)
	if err := w2.Close(); err != nil {
		t.Fatalf("failed to finalize writer 2: %v", err)
	}

	task := &Task{
		InputFiles:      []string{path1, path2},
		FileIDs:         []int{1, 2},
		OutputDirectory: dir,
		NextSegmentID:   100,
		IsBottomLevel:   false,
	}

	res, err := Run(task)
	if err != nil {
		t.Fatalf("compaction run failed: %v", err)
	}

	if len(res.NewFilesCreated) != 1 {
		t.Fatalf("expected 1 output file, got %d", len(res.NewFilesCreated))
	}
	outPath := res.NewFilesCreated[0]

	reader, err := sstable.Open(outPath)
	if err != nil {
		t.Fatalf("failed to open output sstable: %v", err)
	}
	defer reader.Close()

	if reader.EntryCount() != 4 {
		t.Errorf("expected 4 entries in merged sstable, got %d", reader.EntryCount())
	}

	tests := []struct {
		key         string
		wantVal     string
		wantFound   bool
		wantDeleted bool
	}{
		{"a", "v1-new", true, false},
		{"b", "v2", true, false},
		{"c", "", true, true},
		{"d", "v4", true, false},
		{"e", "", false, false},
	}

	for _, tc := range tests {
		val, found, deleted, err := reader.Get([]byte(tc.key))
		if err != nil {
			t.Fatalf("Get(%s) failed: %v", tc.key, err)
		}
		if found != tc.wantFound {
			t.Errorf("key %s: found=%t, want=%t", tc.key, found, tc.wantFound)
		}
		if deleted != tc.wantDeleted {
			t.Errorf("key %s: deleted=%t, want=%t", tc.key, deleted, tc.wantDeleted)
		}
		if tc.wantFound && !tc.wantDeleted && !bytes.Equal(val, []byte(tc.wantVal)) {
			t.Errorf("key %s value mismatch: got %q, want %q", tc.key, val, tc.wantVal)
		}
	}
}

func TestCompactor_BottomLevelTombstoneElision(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 2)
	if err != nil {
		t.Fatalf("failed to create writer 1: %v", err)
	}
	_ = w1.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	_ = w1.Add([]byte("b"), nil, sstable.OpcodeDelete)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer 1: %v", err)
	}

	task := &Task{
		InputFiles:      []string{path1},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   101,
		IsBottomLevel:   true,
	}

	res, err := Run(task)
	if err != nil {
		t.Fatalf("compaction run failed: %v", err)
	}

	outPath := res.NewFilesCreated[0]
	reader, err := sstable.Open(outPath)
	if err != nil {
		t.Fatalf("failed to open output sstable: %v", err)
	}
	defer reader.Close()

	if reader.EntryCount() != 1 {
		t.Errorf("expected 1 entry, got %d", reader.EntryCount())
	}

	_, found, _, _ := reader.Get([]byte("b"))
	if found {
		t.Error("expected tombstone key 'b' to be elided on bottom level compaction")
	}
}

func TestCompactor_ValidateErrors(t *testing.T) {
	task1 := &Task{
		InputFiles: []string{},
	}
	if err := task1.Validate(); err == nil {
		t.Error("expected error for empty input files")
	}

	task2 := &Task{
		InputFiles: []string{"a.sst"},
		FileIDs:    []int{},
	}
	if err := task2.Validate(); err == nil {
		t.Error("expected error for mismatched input files and file IDs lengths")
	}
}

func TestCompactor_Options(t *testing.T) {
	opts := &Options{
		ReadBufferSize: DefaultReaderBufferSize,
		EstimatedKeys:  DefaultEstimatedKeys,
	}
	WithReadBufferSize(-1)(opts)
	if opts.ReadBufferSize != DefaultReaderBufferSize {
		t.Errorf("expected %d, got %d", DefaultReaderBufferSize, opts.ReadBufferSize)
	}
	WithReadBufferSize(4096)(opts)
	if opts.ReadBufferSize != 4096 {
		t.Errorf("expected 4096, got %d", opts.ReadBufferSize)
	}

	WithEstimatedKeys(-1)(opts)
	if opts.EstimatedKeys != DefaultEstimatedKeys {
		t.Errorf("expected %d, got %d", DefaultEstimatedKeys, opts.EstimatedKeys)
	}
	WithEstimatedKeys(500)(opts)
	if opts.EstimatedKeys != 500 {
		t.Errorf("expected 500, got %d", opts.EstimatedKeys)
	}
}

func TestCompactor_OptionsRun(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 1)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = w1.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}

	task := &Task{
		InputFiles:      []string{path1},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   200,
		IsBottomLevel:   false,
	}

	res, err := Run(task, WithReadBufferSize(2048), WithEstimatedKeys(100))
	if err != nil {
		t.Fatalf("compaction run failed: %v", err)
	}
	if len(res.NewFilesCreated) != 1 {
		t.Fatalf("expected 1 output file")
	}
}

func TestCompactor_RunErrors(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 1)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = w1.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}

	task1 := &Task{
		InputFiles:      []string{filepath.Join(dir, "missing.sst")},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   300,
		IsBottomLevel:   false,
	}
	if _, err := Run(task1); err == nil {
		t.Error("expected error for missing input file")
	}

	task2 := &Task{
		InputFiles:      []string{path1},
		FileIDs:         []int{1},
		OutputDirectory: filepath.Join(dir, "nonexistent", "dir"),
		NextSegmentID:   301,
		IsBottomLevel:   false,
	}
	if _, err := Run(task2); err == nil {
		t.Error("expected error for invalid output path/directory")
	}

	pathCorrupt := filepath.Join(dir, "corrupt.sst")
	wCorrupt, err := sstable.NewWriter(pathCorrupt, 1)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = wCorrupt.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	if err := wCorrupt.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}
	corruptBytes, err := os.ReadFile(pathCorrupt)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	binary.LittleEndian.PutUint32(corruptBytes[2:6], 999999)
	if err := os.WriteFile(pathCorrupt, corruptBytes, 0666); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	task3 := &Task{
		InputFiles:      []string{pathCorrupt},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   302,
		IsBottomLevel:   false,
	}
	if _, err := Run(task3); err == nil {
		t.Error("expected error for corrupted input file on iterator initialization")
	}

	pathCorrupt2 := filepath.Join(dir, "corrupt2.sst")
	wCorrupt2, err := sstable.NewWriter(pathCorrupt2, 2)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = wCorrupt2.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	_ = wCorrupt2.Add([]byte("b"), []byte("v2"), sstable.OpcodePut)
	if err := wCorrupt2.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}
	corruptBytes2, err := os.ReadFile(pathCorrupt2)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	binary.LittleEndian.PutUint32(corruptBytes2[12:16], 999999)
	if err := os.WriteFile(pathCorrupt2, corruptBytes2, 0666); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	task4 := &Task{
		InputFiles:      []string{pathCorrupt2},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   303,
		IsBottomLevel:   false,
	}
	if _, err := Run(task4); err == nil {
		t.Error("expected error for corrupted input file during heap advance")
	}
}

