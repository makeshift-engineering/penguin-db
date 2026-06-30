package catalog

// BuildColumnIndexMap creates a mapping from write-time column positions to
// current schema column positions. The returned slice has length
// writeTimeColCount. Each element is the index in currentMeta.Columns
// (considering only non-dropped columns) that corresponds to the write-time
// position, or -1 if the column was dropped.
//
// This mapping is used during schema evolution decoding: when a row was
// written under an older schema, the decoder needs to know which of the
// row's columns map to the current schema and which have been dropped.
//
// The mapping relies on the invariant that columns are never reordered in
// TableMeta.Columns — new columns are always appended, and dropped columns
// retain their index slot with Dropped=true.
func BuildColumnIndexMap(writeTimeColCount int, currentMeta *TableMeta) []int {
	mapping := make([]int, writeTimeColCount)

	// activeIdx tracks the next position in the "active columns only" view.
	activeIdx := 0

	for i := range writeTimeColCount {
		if i >= len(currentMeta.Columns) {
			// The write-time schema had more columns than the current
			// schema's total column list — this shouldn't happen under
			// normal operation but we handle it defensively.
			mapping[i] = -1
			continue
		}

		if currentMeta.Columns[i].Dropped {
			mapping[i] = -1
		} else {
			mapping[i] = activeIdx
			activeIdx++
		}
	}

	return mapping
}

// CountActiveColumns returns the number of non-dropped columns in the
// table metadata.
func CountActiveColumns(meta *TableMeta) int {
	count := 0
	for _, col := range meta.Columns {
		if !col.Dropped {
			count++
		}
	}
	return count
}
