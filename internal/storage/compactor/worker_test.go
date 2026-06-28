package compactor

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// TestCompactor_MergeAndDeduplicate verifies that the compactor successfully merges
// multiple sorted input SSTables, resolves duplicates by keeping newer key versions,
// and correctly handles on-disk tombstone deletion records.
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

// TestCompactor_BottomLevelTombstoneElision verifies that deletion tombstones are elided
// during bottom-level compaction (since there are no older files containing these keys).
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

// TestCompactor_ValidateErrors asserts that task validation correctly flags invalid task bounds
// (such as empty input lists and length mismatches between InputFiles and FileIDs).
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

// TestCompactor_Options verifies that functional options (WithReadBufferSize, WithEstimatedKeys)
// correctly mutate values and safely ignore invalid/negative options.
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

// TestCompactor_OptionsRun verifies that passing WithReadBufferSize and WithEstimatedKeys
// functionally configures compaction runs correctly without errors.
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

// TestCompactor_RunErrors verifies that Run returns errors on input file failures,
// writer creation failures, and data-corruption during heap merge/advance operations.
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
	if err := os.WriteFile(pathCorrupt, corruptBytes, 0o644); err != nil {
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
	if err := os.WriteFile(pathCorrupt2, corruptBytes2, 0o644); err != nil {
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
	outPath := filepath.Join(dir, "000303.sst")
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Error("expected output file to be deleted on compaction failure")
	}
}

// TestCompactor_ValidateOutputDirectory asserts that task validation fails if the output path
// does not exist or points to a file rather than a directory.
func TestCompactor_ValidateOutputDirectory(t *testing.T) {
	dir := t.TempDir()
	task1 := &Task{
		InputFiles:      []string{"a.sst"},
		FileIDs:         []int{1},
		OutputDirectory: filepath.Join(dir, "nonexistent"),
		NextSegmentID:   100,
	}
	if err := task1.Validate(); err == nil {
		t.Error("expected error for non-existent output directory")
	}

	task2 := &Task{
		InputFiles:      []string{"a.sst"},
		FileIDs:         []int{1},
		OutputDirectory: filepath.Join(dir, "input1.sst"),
		NextSegmentID:   100,
	}
	if err := os.WriteFile(task2.OutputDirectory, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := task2.Validate(); err == nil {
		t.Error("expected error for directory path that is a file")
	}
}

// TestCompactor_ValidateFileIDs verifies that task validation correctly returns an error
// if duplicate FileIDs are supplied within the task definition.
func TestCompactor_ValidateFileIDs(t *testing.T) {
	dir := t.TempDir()
	task := &Task{
		InputFiles:      []string{"a.sst", "b.sst"},
		FileIDs:         []int{1, 1},
		OutputDirectory: dir,
		NextSegmentID:   100,
	}
	if err := task.Validate(); err == nil {
		t.Error("expected error for duplicate file IDs")
	}
}

// TestCompactor_SplitSSTables verifies that the compactor successfully splits
// output keys into multiple SSTable files when they cross the MaxSSTableSize threshold.
func TestCompactor_SplitSSTables(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 3)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = w1.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	_ = w1.Add([]byte("b"), []byte("v2"), sstable.OpcodePut)
	_ = w1.Add([]byte("c"), []byte("v3"), sstable.OpcodePut)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}

	task := &Task{
		InputFiles:      []string{path1},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   100,
		IsBottomLevel:   false,
	}

	res, err := Run(task, WithMaxSSTableSize(60), WithEstimatedKeys(3))
	if err != nil {
		t.Fatalf("compaction run failed: %v", err)
	}

	if len(res.NewFilesCreated) != 2 {
		t.Fatalf("expected 2 output files, got %d: %v", len(res.NewFilesCreated), res.NewFilesCreated)
	}

	r0, err := sstable.Open(res.NewFilesCreated[0])
	if err != nil {
		t.Fatalf("failed to open file 0: %v", err)
	}
	defer r0.Close()
	if r0.EntryCount() != 2 {
		t.Errorf("file 0: expected 2 entries, got %d", r0.EntryCount())
	}
	val, found, deleted, _ := r0.Get([]byte("a"))
	if !found || deleted || !bytes.Equal(val, []byte("v1")) {
		t.Error("file 0 does not contain key a")
	}
	val, found, deleted, _ = r0.Get([]byte("b"))
	if !found || deleted || !bytes.Equal(val, []byte("v2")) {
		t.Error("file 0 does not contain key b")
	}

	r1, err := sstable.Open(res.NewFilesCreated[1])
	if err != nil {
		t.Fatalf("failed to open file 1: %v", err)
	}
	defer r1.Close()
	if r1.EntryCount() != 1 {
		t.Errorf("file 1: expected 1 entry, got %d", r1.EntryCount())
	}
	val, found, deleted, _ = r1.Get([]byte("c"))
	if !found || deleted || !bytes.Equal(val, []byte("v3")) {
		t.Error("file 1 does not contain key c")
	}
}

// TestCompactor_SplitErrorsCleanup verifies that if a multi-file compaction run
// encounters an error halfway, all successfully finalized and active SSTables
// created during the run are properly cleaned up from disk.
func TestCompactor_SplitErrorsCleanup(t *testing.T) {
	dir := t.TempDir()

	pathCorrupt := filepath.Join(dir, "corrupt.sst")
	wCorrupt, err := sstable.NewWriter(pathCorrupt, 2)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = wCorrupt.Add([]byte("a"), []byte("v1"), sstable.OpcodePut)
	_ = wCorrupt.Add([]byte("b"), []byte("v2"), sstable.OpcodePut)
	if err := wCorrupt.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}

	corruptBytes, err := os.ReadFile(pathCorrupt)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	binary.LittleEndian.PutUint32(corruptBytes[12:16], 999999)
	if err := os.WriteFile(pathCorrupt, corruptBytes, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	task := &Task{
		InputFiles:      []string{pathCorrupt},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   100,
		IsBottomLevel:   false,
	}

	if _, err := Run(task, WithMaxSSTableSize(50), WithEstimatedKeys(3)); err == nil {
		t.Error("expected error during compaction")
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read output directory: %v", err)
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "00010") {
			t.Errorf("found leftover compaction file: %s", f.Name())
		}
	}
}

// TestCompactor_NoEmptyFiles verifies that bottom-level compaction does not create any
// output SSTable files if all keys are tombstones and thus elided.
func TestCompactor_NoEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "input1.sst")
	w1, err := sstable.NewWriter(path1, 2)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	_ = w1.Add([]byte("a"), nil, sstable.OpcodeDelete)
	_ = w1.Add([]byte("b"), nil, sstable.OpcodeDelete)
	if err := w1.Close(); err != nil {
		t.Fatalf("failed to finalize writer: %v", err)
	}

	task := &Task{
		InputFiles:      []string{path1},
		FileIDs:         []int{1},
		OutputDirectory: dir,
		NextSegmentID:   100,
		IsBottomLevel:   true,
	}

	res, err := Run(task)
	if err != nil {
		t.Fatalf("compaction run failed: %v", err)
	}

	if len(res.NewFilesCreated) != 0 {
		t.Fatalf("expected 0 output files, got %d: %v", len(res.NewFilesCreated), res.NewFilesCreated)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read output directory: %v", err)
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "00010") {
			t.Errorf("found empty/unused output file: %s", f.Name())
		}
	}
}
