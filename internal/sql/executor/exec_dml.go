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

type kvPair struct {
	key   []byte
	value []byte
}

// execInsert executes an INSERT plan by delegating to INSERT ... VALUES or INSERT ... SELECT handlers.
func (e *Executor) execInsert(ctx context.Context, plan *planner.InsertPlan) (*Result, error) {
	if plan.Source != nil {
		return e.execInsertSelect(ctx, plan)
	}
	return e.execInsertValues(ctx, plan)
}

// execInsertValues handles INSERT INTO ... VALUES.
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
		exprValues := make([]any, len(rowExprs))
		for i, expr := range rowExprs {
			v, err := evalExpr(expr, row{})
			if err != nil {
				return nil, fmt.Errorf("executor: evaluating INSERT value: %w", err)
			}
			exprValues[i] = v
		}

		op, err := e.prepareInsertOp(ctx, plan.Database, plan.Table, schema, activeColumns, pkTypes, exprValues, plan.Columns, &nextSeq)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
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

// execInsertSelect handles INSERT INTO ... SELECT.
func (e *Executor) execInsertSelect(ctx context.Context, plan *planner.InsertPlan) (*Result, error) {
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

		op, err := e.prepareInsertOp(ctx, plan.Database, plan.Table, schema, activeColumns, pkTypes, srcRow, plan.Columns, &nextSeq)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
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

// execUpdate executes an UPDATE plan against target table rows.
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

	allPairs, err := e.scanTableKVPairs(ctx, prefix)
	if err != nil {
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

		if plan.Where != nil {
			match, err := evalCond(plan.Where, r)
			if err != nil {
				return nil, err
			}
			if !match {
				continue
			}
		}

		newVals := make([]any, len(vals))
		copy(newVals, vals)
		for _, assign := range plan.Assignments {
			v, err := evalExpr(assign.Value, r)
			if err != nil {
				return nil, fmt.Errorf("executor: evaluating SET value: %w", err)
			}

			col := assign.Column
			if err := validateColumnValue(col.Name, col.Type, col.VarcharLen, col.Nullable, v); err != nil {
				return nil, err
			}
			newVals[col.Index] = v
		}

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

		newKey, err := e.buildRowKey(plan.Database, plan.Table, colValues, schema, activeColumns, pkTypes)
		if err != nil {
			return nil, err
		}

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

// execDelete executes a DELETE plan by deleting matching table rows.
func (e *Executor) execDelete(ctx context.Context, plan *planner.DeletePlan) (*Result, error) {
	schema := plan.Schema
	prefix, err := encoding.EncodeScanPrefix(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding scan prefix: %w", err)
	}

	activeCount := catalog.CountActiveColumns(schema)
	indexMap := catalog.BuildColumnIndexMap(len(schema.Columns), schema)

	candidates, err := e.scanTableKVPairs(ctx, prefix)
	if err != nil {
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

// validateColumnValue checks NOT NULL and VARCHAR max length constraints for a column.
func validateColumnValue(name string, typ ast.DataTypeKind, varcharLen *int, nullable bool, val any) error {
	if val == nil && !nullable {
		return fmt.Errorf("%w: column %q", ErrNotNullViolation, name)
	}
	if typ == ast.TypeVarchar && varcharLen != nil && val != nil {
		if s, ok := val.(string); ok && len(s) > *varcharLen {
			return fmt.Errorf("%w: column %q (max %d, got %d)",
				ErrVarcharTooLong, name, *varcharLen, len(s))
		}
	}
	return nil
}

// scanTableKVPairs performs a prefix scan over KV storage and returns copied key-value pairs.
func (e *Executor) scanTableKVPairs(ctx context.Context, prefix []byte) ([]kvPair, error) {
	iter, err := e.kv.Scan(ctx, prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var pairs []kvPair
	for iter.Valid() {
		key, value := iter.Next()
		if key == nil || value == nil {
			continue
		}
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		pairs = append(pairs, kvPair{key: keyCopy, value: valueCopy})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

// buildRowKey extracts primary key column values and encodes the storage row key db.table.pkBytes.
func (e *Executor) buildRowKey(db, table string, colValues []codec.ColumnValue, schema *catalog.TableMeta, activeColumns []catalog.ColumnMeta, pkTypes []ast.DataTypeKind) ([]byte, error) {
	pkVals, err := e.extractPKValues(colValues, schema, activeColumns)
	if err != nil {
		return nil, err
	}
	pkBytes, err := encoding.EncodePK(pkTypes, pkVals)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding PK: %w", err)
	}
	rowKey, err := encoding.EncodeRowKey(db, table, pkBytes)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding row key: %w", err)
	}
	return rowKey, nil
}

// prepareInsertOp validates column values, encodes a row, checks for duplicate primary keys, and returns a put Op.
func (e *Executor) prepareInsertOp(ctx context.Context, db, table string, schema *catalog.TableMeta, activeColumns []catalog.ColumnMeta, pkTypes []ast.DataTypeKind, rowValues []any, targetCols []planner.ResolvedColumn, nextSeq *uint64) (kv.Op, error) {
	colValues := make([]codec.ColumnValue, len(activeColumns))

	for i, col := range targetCols {
		val := rowValues[i]
		if err := validateColumnValue(col.Name, col.Type, col.VarcharLen, col.Nullable, val); err != nil {
			return kv.Op{}, err
		}
		cv, err := anyToColumnValue(val, col.Type)
		if err != nil {
			return kv.Op{}, fmt.Errorf("executor: converting value for column %q: %w", col.Name, err)
		}
		colValues[col.Index] = cv
	}

	if schema.HasSnowflakeID {
		*nextSeq++
		colValues[0] = codec.BigIntValue(int64(*nextSeq))
	}

	for i := range colValues {
		if colValues[i].Type == 0 && colValues[i].Raw == nil && !colValues[i].IsNull {
			colValues[i] = codec.NullValue(activeColumns[i].Type)
		}
	}

	codecRow := &codec.Row{Values: colValues}
	encoded, err := codec.Encode(codecRow)
	if err != nil {
		return kv.Op{}, fmt.Errorf("executor: encoding row: %w", err)
	}

	rowKey, err := e.buildRowKey(db, table, colValues, schema, activeColumns, pkTypes)
	if err != nil {
		return kv.Op{}, err
	}

	_, err = e.kv.Get(ctx, rowKey)
	if err == nil {
		return kv.Op{}, fmt.Errorf("%w: key already exists in %q.%q", ErrDuplicateKey, db, table)
	}
	if !errors.Is(err, kv.ErrKeyNotFound) {
		return kv.Op{}, fmt.Errorf("executor: checking PK existence: %w", err)
	}

	return kv.Op{Type: kv.OpPut, Key: rowKey, Value: encoded}, nil
}

// extractPKValues extracts primary key column values from a row's codec.ColumnValue slice.
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
