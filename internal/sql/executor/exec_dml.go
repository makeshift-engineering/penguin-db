package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/codec"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/encoding"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// execInsert executes an INSERT plan.
func (e *Executor) execInsert(ctx context.Context, plan *planner.InsertPlan) (*Result, error) {
	if plan.Source != nil {
		return e.execInsertSelect(ctx, plan)
	}
	return e.execInsertValues(ctx, plan)
}

// execInsertValues handles INSERT ... VALUES.
func (e *Executor) execInsertValues(ctx context.Context, plan *planner.InsertPlan) (*Result, error) {
	schema := plan.Schema
	activeColumns := schema.ActiveColumns()

	pkTypes, err := e.catalog.PKColumnTypes(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: getting PK types: %w", err)
	}

	// Handle snowflake ID sequence if needed.
	var nextSeq uint64
	if schema.HasSnowflakeID {
		nextSeq, err = e.readSequence(ctx, plan.Database, plan.Table)
		if err != nil {
			return nil, err
		}
	}

	var ops []kv.Op

	for _, rowExprs := range plan.Rows {
		// Build the full row of column values.
		colValues := make([]codec.ColumnValue, len(activeColumns))

		// Evaluate each expression in the VALUES row.
		exprValues := make([]any, len(rowExprs))
		for i, expr := range rowExprs {
			v, err := evalExpr(expr, row{})
			if err != nil {
				return nil, fmt.Errorf("executor: evaluating INSERT value: %w", err)
			}
			exprValues[i] = v
		}

		// Map expression values to column positions.
		for i, col := range plan.Columns {
			val := exprValues[i]

			// Validate NOT NULL constraint.
			if val == nil && !col.Nullable {
				return nil, fmt.Errorf("%w: column %q", ErrNotNullViolation, col.Name)
			}

			// Validate VARCHAR length.
			if col.Type == ast.TypeVarchar && col.VarcharLen != nil && val != nil {
				if s, ok := val.(string); ok && len(s) > *col.VarcharLen {
					return nil, fmt.Errorf("%w: column %q (max %d, got %d)",
						ErrVarcharTooLong, col.Name, *col.VarcharLen, len(s))
				}
			}

			cv, err := anyToColumnValue(val, col.Type)
			if err != nil {
				return nil, fmt.Errorf("executor: converting value for column %q: %w", col.Name, err)
			}
			colValues[col.Index] = cv
		}

		// Fill in snowflake ID if needed.
		if schema.HasSnowflakeID {
			nextSeq++
			colValues[0] = codec.BigIntValue(int64(nextSeq))
		}

		// Fill in default NULL for any unset columns.
		for i := range colValues {
			if colValues[i].Type == 0 && colValues[i].Raw == nil && !colValues[i].IsNull {
				colValues[i] = codec.NullValue(activeColumns[i].Type)
			}
		}

		// Encode the row.
		codecRow := &codec.Row{Values: colValues}
		encoded, err := codec.Encode(codecRow)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding row: %w", err)
		}

		// Build the primary key.
		pkVals, err := e.extractPKValues(colValues, schema, activeColumns)
		if err != nil {
			return nil, err
		}
		pkBytes, err := encoding.EncodePK(pkTypes, pkVals)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding PK: %w", err)
		}
		rowKey, err := encoding.EncodeRowKey(plan.Database, plan.Table, pkBytes)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding row key: %w", err)
		}

		// Check for duplicate PK.
		_, err = e.kv.Get(ctx, rowKey)
		if err == nil {
			return nil, fmt.Errorf("%w: key already exists in %q.%q", ErrDuplicateKey, plan.Database, plan.Table)
		}
		if !errors.Is(err, kv.ErrKeyNotFound) {
			return nil, fmt.Errorf("executor: checking PK existence: %w", err)
		}

		ops = append(ops, kv.Op{Type: kv.OpPut, Key: rowKey, Value: encoded})
	}

	// Update sequence counter if we used snowflake IDs.
	if schema.HasSnowflakeID {
		seqKey, err := encoding.EncodeCatalogSeqKey(plan.Database, plan.Table)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding seq key: %w", err)
		}
		ops = append(ops, kv.Op{Type: kv.OpPut, Key: seqKey, Value: encodeSequenceValue(nextSeq)})
	}

	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing INSERT: %w", err)
	}

	return &Result{
		Type:         ResultDML,
		RowsAffected: int64(len(plan.Rows)),
	}, nil
}

// execInsertSelect handles INSERT ... SELECT.
func (e *Executor) execInsertSelect(ctx context.Context, plan *planner.InsertPlan) (*Result, error) {
	// Execute the source query.
	queryResult, err := e.execQuery(ctx, plan.Source)
	if err != nil {
		return nil, fmt.Errorf("executor: executing INSERT ... SELECT source: %w", err)
	}

	schema := plan.Schema
	activeColumns := schema.ActiveColumns()

	pkTypes, err := e.catalog.PKColumnTypes(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: getting PK types: %w", err)
	}

	var nextSeq uint64
	if schema.HasSnowflakeID {
		nextSeq, err = e.readSequence(ctx, plan.Database, plan.Table)
		if err != nil {
			return nil, err
		}
	}

	var ops []kv.Op

	for _, srcRow := range queryResult.Rows {
		if len(srcRow) != len(plan.Columns) {
			return nil, fmt.Errorf("%w: SELECT produces %d columns, INSERT expects %d",
				ErrColumnCountMismatch, len(srcRow), len(plan.Columns))
		}

		colValues := make([]codec.ColumnValue, len(activeColumns))

		for i, col := range plan.Columns {
			val := srcRow[i]

			if val == nil && !col.Nullable {
				return nil, fmt.Errorf("%w: column %q", ErrNotNullViolation, col.Name)
			}

			cv, err := anyToColumnValue(val, col.Type)
			if err != nil {
				return nil, fmt.Errorf("executor: converting value for column %q: %w", col.Name, err)
			}
			colValues[col.Index] = cv
		}

		if schema.HasSnowflakeID {
			nextSeq++
			colValues[0] = codec.BigIntValue(int64(nextSeq))
		}

		for i := range colValues {
			if colValues[i].Type == 0 && colValues[i].Raw == nil && !colValues[i].IsNull {
				colValues[i] = codec.NullValue(activeColumns[i].Type)
			}
		}

		codecRow := &codec.Row{Values: colValues}
		encoded, err := codec.Encode(codecRow)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding row: %w", err)
		}

		pkVals, err := e.extractPKValues(colValues, schema, activeColumns)
		if err != nil {
			return nil, err
		}
		pkBytes, err := encoding.EncodePK(pkTypes, pkVals)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding PK: %w", err)
		}
		rowKey, err := encoding.EncodeRowKey(plan.Database, plan.Table, pkBytes)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding row key: %w", err)
		}

		_, err = e.kv.Get(ctx, rowKey)
		if err == nil {
			return nil, fmt.Errorf("%w: key already exists in %q.%q", ErrDuplicateKey, plan.Database, plan.Table)
		}
		if !errors.Is(err, kv.ErrKeyNotFound) {
			return nil, fmt.Errorf("executor: checking PK existence: %w", err)
		}

		ops = append(ops, kv.Op{Type: kv.OpPut, Key: rowKey, Value: encoded})
	}

	if schema.HasSnowflakeID {
		seqKey, err := encoding.EncodeCatalogSeqKey(plan.Database, plan.Table)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding seq key: %w", err)
		}
		ops = append(ops, kv.Op{Type: kv.OpPut, Key: seqKey, Value: encodeSequenceValue(nextSeq)})
	}

	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing INSERT ... SELECT: %w", err)
	}

	return &Result{
		Type:         ResultDML,
		RowsAffected: int64(len(queryResult.Rows)),
	}, nil
}

// execUpdate executes an UPDATE plan.
func (e *Executor) execUpdate(ctx context.Context, plan *planner.UpdatePlan) (*Result, error) {
	schema := plan.Schema
	activeColumns := schema.ActiveColumns()

	prefix, err := encoding.EncodeScanPrefix(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding scan prefix: %w", err)
	}

	pkTypes, err := e.catalog.PKColumnTypes(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: getting PK types: %w", err)
	}

	activeCount := catalog.CountActiveColumns(schema)
	indexMap := catalog.BuildColumnIndexMap(len(schema.Columns), schema)

	iter, err := e.kv.Scan(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("executor: scanning for UPDATE: %w", err)
	}

	type kvPair struct {
		key   []byte
		value []byte
	}
	var allPairs []kvPair
	for iter.Valid() {
		key, value := iter.Next()
		if key == nil || value == nil {
			continue
		}
		// Copy key and value since the iterator may reuse buffers.
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		allPairs = append(allPairs, kvPair{key: keyCopy, value: valueCopy})
	}
	iter.Close()
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("executor: scanning for UPDATE: %w", err)
	}

	var ops []kv.Op
	var affected int64

	for _, pair := range allPairs {
		encoded, err := codec.Decode(pair.value)
		if err != nil {
			return nil, fmt.Errorf("executor: decoding row: %w", err)
		}

		vals, err := decodeRowToAny(encoded, activeCount, indexMap)
		if err != nil {
			return nil, fmt.Errorf("executor: converting row: %w", err)
		}

		r := row{values: vals}

		// Apply WHERE filter.
		if plan.Where != nil {
			match, err := evalCond(plan.Where, r)
			if err != nil {
				return nil, err
			}
			if !match {
				continue
			}
		}

		// Apply assignments.
		newVals := make([]any, len(vals))
		copy(newVals, vals)
		for _, assign := range plan.Assignments {
			v, err := evalExpr(assign.Value, r)
			if err != nil {
				return nil, fmt.Errorf("executor: evaluating SET value: %w", err)
			}

			col := assign.Column
			if v == nil && !col.Nullable {
				return nil, fmt.Errorf("%w: column %q", ErrNotNullViolation, col.Name)
			}
			if col.Type == ast.TypeVarchar && col.VarcharLen != nil && v != nil {
				if s, ok := v.(string); ok && len(s) > *col.VarcharLen {
					return nil, fmt.Errorf("%w: column %q (max %d, got %d)",
						ErrVarcharTooLong, col.Name, *col.VarcharLen, len(s))
				}
			}
			newVals[col.Index] = v
		}

		// Re-encode the row.
		colValues := make([]codec.ColumnValue, len(activeColumns))
		for i, col := range activeColumns {
			cv, err := anyToColumnValue(newVals[i], col.Type)
			if err != nil {
				return nil, fmt.Errorf("executor: converting updated value for column %q: %w", col.Name, err)
			}
			colValues[i] = cv
		}

		codecRow := &codec.Row{Values: colValues}
		newEncoded, err := codec.Encode(codecRow)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding updated row: %w", err)
		}

		// Compute the new PK.
		pkVals, err := e.extractPKValues(colValues, schema, activeColumns)
		if err != nil {
			return nil, err
		}
		pkBytes, err := encoding.EncodePK(pkTypes, pkVals)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding PK: %w", err)
		}
		newKey, err := encoding.EncodeRowKey(plan.Database, plan.Table, pkBytes)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding row key: %w", err)
		}

		// If the PK changed, delete the old key.
		if string(newKey) != string(pair.key) {
			ops = append(ops, kv.Op{Type: kv.OpDelete, Key: pair.key})
		}
		ops = append(ops, kv.Op{Type: kv.OpPut, Key: newKey, Value: newEncoded})
		affected++
	}

	if len(ops) > 0 {
		if err := e.kv.WriteBatch(ctx, ops); err != nil {
			return nil, fmt.Errorf("executor: writing UPDATE: %w", err)
		}
	}

	return &Result{
		Type:         ResultDML,
		RowsAffected: affected,
	}, nil
}

// execDelete executes a DELETE plan.
func (e *Executor) execDelete(ctx context.Context, plan *planner.DeletePlan) (*Result, error) {
	schema := plan.Schema
	prefix, err := encoding.EncodeScanPrefix(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding scan prefix: %w", err)
	}

	activeCount := catalog.CountActiveColumns(schema)
	indexMap := catalog.BuildColumnIndexMap(len(schema.Columns), schema)

	iter, err := e.kv.Scan(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("executor: scanning for DELETE: %w", err)
	}

	type kvKey struct {
		key []byte
	}
	var candidates []struct {
		key   []byte
		value []byte
	}
	for iter.Valid() {
		key, value := iter.Next()
		if key == nil {
			continue
		}
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		candidates = append(candidates, struct {
			key   []byte
			value []byte
		}{key: keyCopy, value: valueCopy})
	}
	iter.Close()
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("executor: scanning for DELETE: %w", err)
	}

	var ops []kv.Op
	var affected int64

	for _, c := range candidates {
		if plan.Where != nil {
			encoded, err := codec.Decode(c.value)
			if err != nil {
				return nil, fmt.Errorf("executor: decoding row: %w", err)
			}
			vals, err := decodeRowToAny(encoded, activeCount, indexMap)
			if err != nil {
				return nil, fmt.Errorf("executor: converting row: %w", err)
			}
			match, err := evalCond(plan.Where, row{values: vals})
			if err != nil {
				return nil, err
			}
			if !match {
				continue
			}
		}
		ops = append(ops, kv.Op{Type: kv.OpDelete, Key: c.key})
		affected++
	}

	if len(ops) > 0 {
		if err := e.kv.WriteBatch(ctx, ops); err != nil {
			return nil, fmt.Errorf("executor: writing DELETE: %w", err)
		}
	}

	return &Result{
		Type:         ResultDML,
		RowsAffected: affected,
	}, nil
}

// extractPKValues extracts primary key column values from a row's
// codec.ColumnValue slice, converting them to the types EncodePK expects.
func (e *Executor) extractPKValues(colValues []codec.ColumnValue, schema *catalog.TableMeta, activeColumns []catalog.ColumnMeta) ([]any, error) {
	pkColNames := schema.PrimaryKey
	if schema.HasSnowflakeID {
		pkColNames = []string{"id"}
	}

	pkVals := make([]any, len(pkColNames))
	for i, pkName := range pkColNames {
		// Find the active column index for this PK column.
		found := false
		for j, col := range activeColumns {
			if col.Name == pkName {
				v, err := columnValueToAny(colValues[j])
				if err != nil {
					return nil, fmt.Errorf("executor: reading PK column %q: %w", pkName, err)
				}
				if v == nil {
					return nil, fmt.Errorf("%w: column %q", ErrNullPrimaryKey, pkName)
				}
				pkv, err := anyToPKValue(v, col.Type)
				if err != nil {
					return nil, err
				}
				pkVals[i] = pkv
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("executor: PK column %q not found in active columns", pkName)
		}
	}
	return pkVals, nil
}

// readSequence reads the current sequence counter value for a table.
func (e *Executor) readSequence(ctx context.Context, db, table string) (uint64, error) {
	seqKey, err := encoding.EncodeCatalogSeqKey(db, table)
	if err != nil {
		return 0, fmt.Errorf("executor: encoding seq key: %w", err)
	}
	seqBytes, err := e.kv.Get(ctx, seqKey)
	if errors.Is(err, kv.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("executor: reading seq counter: %w", err)
	}
	return decodeSequenceValue(seqBytes), nil
}
