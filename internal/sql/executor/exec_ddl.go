package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/encoding"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// execCreateDatabase executes a CREATE DATABASE plan.
func (e *Executor) execCreateDatabase(ctx context.Context, plan *planner.CreateDatabasePlan) (*Result, error) {
	if plan.NoOp {
		return &Result{
			Type:    ResultDDL,
			Message: fmt.Sprintf("database %q already exists (IF NOT EXISTS)", plan.Name),
		}, nil
	}

	meta := &catalog.DatabaseMeta{
		Name:      plan.Name,
		CreatedAt: time.Now(),
	}
	ops, err := catalog.BuildCreateDatabaseOps(meta)
	if err != nil {
		return nil, fmt.Errorf("executor: building CREATE DATABASE ops: %w", err)
	}
	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing CREATE DATABASE: %w", err)
	}
	e.catalog.ApplyCreateDatabase(meta)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("database %q created", plan.Name),
	}, nil
}

// execUseDatabase executes a USE statement. No KV writes — just signals
// the caller to update the session's active database.
func (e *Executor) execUseDatabase(_ context.Context, plan *planner.UseDatabasePlan) (*Result, error) {
	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("database changed to %q", plan.Name),
		SessionUpdate: &SessionUpdate{
			ActiveDatabase: plan.Name,
		},
	}, nil
}

// execDropDatabase executes a DROP DATABASE plan.
func (e *Executor) execDropDatabase(ctx context.Context, plan *planner.DropDatabasePlan) (*Result, error) {
	if plan.NoOp {
		return &Result{
			Type:    ResultDDL,
			Message: fmt.Sprintf("database %q does not exist (IF EXISTS)", plan.Name),
		}, nil
	}

	ops, err := catalog.BuildDropDatabaseOps(plan.Name, plan.Tables)
	if err != nil {
		return nil, fmt.Errorf("executor: building DROP DATABASE ops: %w", err)
	}

	// Delete all user data rows for each table in the database.
	for _, t := range plan.Tables {
		ops, err = e.appendTableDeleteOps(ctx, ops, plan.Name, t.Name)
		if err != nil {
			return nil, err
		}
	}

	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing DROP DATABASE: %w", err)
	}
	e.catalog.ApplyDropDatabase(plan.Name)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("database %q dropped", plan.Name),
	}, nil
}

// execCreateTable executes a CREATE TABLE plan.
func (e *Executor) execCreateTable(ctx context.Context, plan *planner.CreateTablePlan) (*Result, error) {
	if plan.NoOp {
		return &Result{
			Type:    ResultDDL,
			Message: fmt.Sprintf("table %q already exists (IF NOT EXISTS)", plan.Table),
		}, nil
	}

	ops, err := catalog.BuildCreateTableOps(e.catalog, plan.Schema)
	if err != nil {
		return nil, fmt.Errorf("executor: building CREATE TABLE ops: %w", err)
	}

	// If the table has a snowflake ID, initialise the sequence counter to 0.
	if plan.Schema.HasSnowflakeID {
		seqKey, err := encoding.EncodeCatalogSeqKey(plan.Database, plan.Table)
		if err != nil {
			return nil, fmt.Errorf("executor: encoding seq key: %w", err)
		}
		ops = append(ops, kv.Op{
			Type:  kv.OpPut,
			Key:   seqKey,
			Value: encodeSequenceValue(0),
		})
	}

	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing CREATE TABLE: %w", err)
	}
	e.catalog.ApplyCreateTable(plan.Schema)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("table %q.%q created", plan.Database, plan.Table),
	}, nil
}

// execDropTable executes a DROP TABLE plan.
func (e *Executor) execDropTable(ctx context.Context, plan *planner.DropTablePlan) (*Result, error) {
	if plan.NoOp {
		return &Result{
			Type:    ResultDDL,
			Message: fmt.Sprintf("table %q does not exist (IF EXISTS)", plan.Table),
		}, nil
	}

	ops, err := catalog.BuildDropTableOps(plan.Database, plan.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: building DROP TABLE ops: %w", err)
	}

	// Delete all user data rows for the table.
	ops, err = e.appendTableDeleteOps(ctx, ops, plan.Database, plan.Table)
	if err != nil {
		return nil, err
	}

	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing DROP TABLE: %w", err)
	}
	e.catalog.ApplyDropTable(plan.Database, plan.Table)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("table %q.%q dropped", plan.Database, plan.Table),
	}, nil
}

// execAlterTable executes an ALTER TABLE plan (add/modify/drop column).
func (e *Executor) execAlterTable(ctx context.Context, plan *planner.AlterTablePlan) (*Result, error) {
	ops, finalMeta, err := catalog.BuildAlterTableOps(plan.OldSchema, plan.NewSchema)
	if err != nil {
		return nil, fmt.Errorf("executor: building ALTER TABLE ops: %w", err)
	}
	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing ALTER TABLE: %w", err)
	}
	e.catalog.ApplyAlterTable(finalMeta)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("table %q.%q altered", finalMeta.Database, finalMeta.Name),
	}, nil
}

// execRenameTable executes an ALTER TABLE ... RENAME TO plan.
func (e *Executor) execRenameTable(ctx context.Context, plan *planner.RenameTablePlan) (*Result, error) {
	// Read the current sequence counter value, if any.
	seqKey, err := encoding.EncodeCatalogSeqKey(plan.Database, plan.OldName)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding seq key: %w", err)
	}
	var seqValue []byte
	seqValue, err = e.kv.Get(ctx, seqKey)
	if err != nil && !errors.Is(err, kv.ErrKeyNotFound) {
		return nil, fmt.Errorf("executor: reading seq counter: %w", err)
	}

	ops, err := catalog.BuildRenameTableOps(plan.Database, plan.OldName, plan.NewName, plan.Schema, seqValue)
	if err != nil {
		return nil, fmt.Errorf("executor: building RENAME TABLE ops: %w", err)
	}
	if err := e.kv.WriteBatch(ctx, ops); err != nil {
		return nil, fmt.Errorf("executor: writing RENAME TABLE: %w", err)
	}
	e.catalog.ApplyRenameTable(plan.Database, plan.OldName, plan.NewName, plan.Schema)

	return &Result{
		Type:    ResultDDL,
		Message: fmt.Sprintf("table %q.%q renamed to %q", plan.Database, plan.OldName, plan.NewName),
	}, nil
}

// appendTableDeleteOps scans all user data row keys for db.table and appends kv.OpDelete operations to ops.
func (e *Executor) appendTableDeleteOps(ctx context.Context, ops []kv.Op, db, table string) ([]kv.Op, error) {
	prefix, err := encoding.EncodeScanPrefix(db, table)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding scan prefix for %q.%q: %w", db, table, err)
	}
	iter, err := e.kv.Scan(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("executor: scanning rows for %q.%q: %w", db, table, err)
	}
	defer iter.Close()

	for iter.Valid() {
		key, _ := iter.Next()
		if key != nil {
			ops = append(ops, kv.Op{Type: kv.OpDelete, Key: key})
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("executor: scanning rows for %q.%q: %w", db, table, err)
	}
	return ops, nil
}
