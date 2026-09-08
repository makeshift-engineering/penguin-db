package executor

import (
	"context"
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// Executor translates validated planner.Plan trees into concrete KV
// operations. It holds a reference to the KV store and the in-memory
// catalog. One Executor can safely serve many concurrent requests: each
// Execute call is self-contained and does not mutate the Executor itself.
type Executor struct {
	kv      kv.KV
	catalog *catalog.Catalog
}

// New creates an Executor backed by the given KV store and catalog.
func New(store kv.KV, cat *catalog.Catalog) *Executor {
	return &Executor{kv: store, catalog: cat}
}

// Execute dispatches a plan to the appropriate execution method and
// returns the result. The context is threaded through to all KV
// operations for cancellation and deadline support.
func (e *Executor) Execute(ctx context.Context, plan planner.Plan) (*Result, error) {
	switch p := plan.(type) {
	case *planner.CreateDatabasePlan:
		return e.execCreateDatabase(ctx, p)
	case *planner.UseDatabasePlan:
		return e.execUseDatabase(ctx, p)
	case *planner.DropDatabasePlan:
		return e.execDropDatabase(ctx, p)
	case *planner.CreateTablePlan:
		return e.execCreateTable(ctx, p)
	case *planner.DropTablePlan:
		return e.execDropTable(ctx, p)
	case *planner.AlterTablePlan:
		return e.execAlterTable(ctx, p)
	case *planner.RenameTablePlan:
		return e.execRenameTable(ctx, p)
	case *planner.QueryPlan:
		return e.execQuery(ctx, p)
	case *planner.InsertPlan:
		return e.execInsert(ctx, p)
	case *planner.UpdatePlan:
		return e.execUpdate(ctx, p)
	case *planner.DeletePlan:
		return e.execDelete(ctx, p)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedPlan, plan)
	}
}
