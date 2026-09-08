package executor

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/codec"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/encoding"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// execQuery executes a SELECT plan by recursively walking the RelNode tree.
func (e *Executor) execQuery(ctx context.Context, plan *planner.QueryPlan) (*Result, error) {
	rows, err := e.execRelNode(ctx, plan.Root)
	if err != nil {
		return nil, err
	}

	// Convert internal rows to the output format.
	resultRows := make([][]any, len(rows))
	for i, r := range rows {
		resultRows[i] = r.values
	}

	columns := make([]ColumnInfo, len(plan.Columns))
	for i, c := range plan.Columns {
		columns[i] = ColumnInfo{Name: c.Name, Type: c.Type}
	}

	return &Result{
		Type:    ResultQuery,
		Columns: columns,
		Rows:    resultRows,
	}, nil
}

// execRelNode recursively evaluates a relational operator node and returns
// the resulting row set.
func (e *Executor) execRelNode(ctx context.Context, node planner.RelNode) ([]row, error) {
	if node == nil {
		// A nil input (e.g. ProjectNode with nil Input for FROM-less
		// queries like SELECT 1+1) produces a single empty row.
		return []row{{values: nil}}, nil
	}

	switch n := node.(type) {
	case *planner.ScanNode:
		return e.execScan(ctx, n)
	case *planner.FilterNode:
		return e.execFilter(ctx, n)
	case *planner.ProjectNode:
		return e.execProject(ctx, n)
	case *planner.JoinNode:
		return e.execJoin(ctx, n)
	case *planner.AggregateNode:
		return e.execAggregate(ctx, n)
	case *planner.SortNode:
		return e.execSort(ctx, n)
	case *planner.LimitNode:
		return e.execLimit(ctx, n)
	case *planner.DistinctNode:
		return e.execDistinct(ctx, n)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedRelNode, node)
	}
}

// execScan reads all rows from a table by prefix scanning the KV store.
func (e *Executor) execScan(ctx context.Context, node *planner.ScanNode) ([]row, error) {
	rt := node.Table
	prefix, err := encoding.EncodeScanPrefix(rt.Database, rt.Table)
	if err != nil {
		return nil, fmt.Errorf("executor: encoding scan prefix for %q.%q: %w", rt.Database, rt.Table, err)
	}

	iter, err := e.kv.Scan(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("executor: scanning %q.%q: %w", rt.Database, rt.Table, err)
	}
	defer iter.Close()

	activeCount := catalog.CountActiveColumns(rt.Schema)
	indexMap := catalog.BuildColumnIndexMap(len(rt.Schema.Columns), rt.Schema)

	var rows []row
	for iter.Valid() {
		_, value := iter.Next()
		if value == nil {
			continue
		}

		encoded, err := codec.Decode(value)
		if err != nil {
			return nil, fmt.Errorf("executor: decoding row from %q.%q: %w", rt.Database, rt.Table, err)
		}

		vals, err := decodeRowToAny(encoded, activeCount, indexMap)
		if err != nil {
			return nil, fmt.Errorf("executor: converting row from %q.%q: %w", rt.Database, rt.Table, err)
		}

		rows = append(rows, row{values: vals})
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("executor: scanning %q.%q: %w", rt.Database, rt.Table, err)
	}

	return rows, nil
}

// execFilter evaluates Input and keeps only rows for which Cond is true.
func (e *Executor) execFilter(ctx context.Context, node *planner.FilterNode) ([]row, error) {
	input, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	var result []row
	for _, r := range input {
		match, err := evalCond(node.Cond, r)
		if err != nil {
			return nil, err
		}
		if match {
			result = append(result, r)
		}
	}
	return result, nil
}

// execProject evaluates Items against each row of Input.
func (e *Executor) execProject(ctx context.Context, node *planner.ProjectNode) ([]row, error) {
	input, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	var result []row
	for _, r := range input {
		vals := make([]any, len(node.Items))
		for i, item := range node.Items {
			v, err := evalExpr(item.Expr, r)
			if err != nil {
				return nil, fmt.Errorf("executor: evaluating projection item %d: %w", i, err)
			}
			vals[i] = v
		}
		result = append(result, row{values: vals})
	}
	return result, nil
}

// execJoin performs a nested-loop join (INNER or CROSS).
func (e *Executor) execJoin(ctx context.Context, node *planner.JoinNode) ([]row, error) {
	left, err := e.execRelNode(ctx, node.Left)
	if err != nil {
		return nil, err
	}
	right, err := e.execRelNode(ctx, node.Right)
	if err != nil {
		return nil, err
	}

	var result []row
	for _, lr := range left {
		for _, rr := range right {
			// Concatenate the two rows.
			combined := make([]any, len(lr.values)+len(rr.values))
			copy(combined, lr.values)
			copy(combined[len(lr.values):], rr.values)
			jr := row{values: combined}

			if node.On != nil {
				match, err := evalCond(node.On, jr)
				if err != nil {
					return nil, err
				}
				if !match {
					continue
				}
			}
			result = append(result, jr)
		}
	}
	return result, nil
}

// execAggregate groups rows by GroupBy keys and evaluates aggregate
// functions.
func (e *Executor) execAggregate(ctx context.Context, node *planner.AggregateNode) ([]row, error) {
	input, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	// Group rows by their GroupBy key values.
	type group struct {
		keyValues []any
		rows      []row
	}

	var groups []group
	groupIndex := make(map[string]int) // serialized key → index into groups

	for _, r := range input {
		var keyParts []any
		for _, gbExpr := range node.GroupBy {
			v, err := evalExpr(gbExpr, r)
			if err != nil {
				return nil, err
			}
			keyParts = append(keyParts, v)
		}
		keyStr := fmt.Sprintf("%v", keyParts)
		if idx, ok := groupIndex[keyStr]; ok {
			groups[idx].rows = append(groups[idx].rows, r)
		} else {
			groupIndex[keyStr] = len(groups)
			groups = append(groups, group{keyValues: keyParts, rows: []row{r}})
		}
	}

	// If there are no groups (empty input with no GROUP BY), produce one
	// empty group for aggregate-without-GROUP-BY (e.g. SELECT COUNT(*) FROM empty_table).
	if len(groups) == 0 && len(node.GroupBy) == 0 {
		groups = append(groups, group{rows: nil})
	}

	// Evaluate aggregates for each group.
	var result []row
	for _, g := range groups {
		vals := make([]any, len(node.Aggregates))
		for i, agg := range node.Aggregates {
			v, err := evalAggregateExpr(agg.Expr, g.rows)
			if err != nil {
				return nil, err
			}
			vals[i] = v
		}
		result = append(result, row{values: vals})
	}
	return result, nil
}

// evalAggregateExpr evaluates an expression within an aggregate context.
// If the expression is an aggregate function call, it computes the
// aggregate over the group's rows. Otherwise (e.g. a GROUP BY key
// reference or a literal), it evaluates against the first row of the
// group.
func evalAggregateExpr(expr planner.ResolvedExpr, groupRows []row) (any, error) {
	switch e := expr.(type) {
	case *planner.ResolvedFunctionCall:
		return evalAggregateFunc(e, groupRows)
	case *planner.ResolvedBinaryExpr:
		// Composite expressions may contain aggregates in sub-trees.
		left, err := evalAggregateExpr(e.Left, groupRows)
		if err != nil {
			return nil, err
		}
		right, err := evalAggregateExpr(e.Right, groupRows)
		if err != nil {
			return nil, err
		}
		if left == nil || right == nil {
			return nil, nil
		}
		lf, err := toFloat64(left)
		if err != nil {
			return nil, err
		}
		rf, err := toFloat64(right)
		if err != nil {
			return nil, err
		}
		var result float64
		switch e.Op {
		case planner.OpAdd:
			result = lf + rf
		case planner.OpSub:
			result = lf - rf
		case planner.OpMul:
			result = lf * rf
		case planner.OpDiv:
			if rf == 0 {
				return nil, ErrDivisionByZero
			}
			result = lf / rf
		case planner.OpMod:
			if rf == 0 {
				return nil, ErrDivisionByZero
			}
			result = math.Mod(lf, rf)
		default:
			return nil, fmt.Errorf("unsupported operator in aggregate expression")
		}
		return coerceResultByKind(result, e.ResolvedType()), nil
	case *planner.ResolvedUnaryExpr:
		operand, err := evalAggregateExpr(e.Operand, groupRows)
		if err != nil {
			return nil, err
		}
		if operand == nil {
			return nil, nil
		}
		f, err := toFloat64(operand)
		if err != nil {
			return nil, err
		}
		return coerceResultByKind(-f, e.ResolvedType()), nil
	default:
		// For non-aggregate expressions (column refs, literals), evaluate
		// against the first row. If the group is empty, return nil.
		if len(groupRows) == 0 {
			return nil, nil
		}
		return evalExpr(expr, groupRows[0])
	}
}

// evalAggregateFunc computes an aggregate function over a group of rows.
func evalAggregateFunc(fn *planner.ResolvedFunctionCall, groupRows []row) (any, error) {
	switch fn.Name {
	case "COUNT":
		return evalCount(fn, groupRows)
	case "SUM":
		return evalSum(fn, groupRows)
	case "AVG":
		return evalAvg(fn, groupRows)
	case "MIN":
		return evalMinMax(fn, groupRows, false)
	case "MAX":
		return evalMinMax(fn, groupRows, true)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAggFunc, fn.Name)
	}
}

// evalCount implements COUNT(*) and COUNT([DISTINCT] expr).
func evalCount(fn *planner.ResolvedFunctionCall, groupRows []row) (any, error) {
	if fn.Star {
		return int64(len(groupRows)), nil
	}

	if fn.Distinct {
		seen := make(map[any]struct{})
		for _, r := range groupRows {
			v, err := evalExpr(fn.Args[0], r)
			if err != nil {
				return nil, err
			}
			if v == nil {
				continue
			}
			seen[v] = struct{}{}
		}
		return int64(len(seen)), nil
	}

	var count int64
	for _, r := range groupRows {
		v, err := evalExpr(fn.Args[0], r)
		if err != nil {
			return nil, err
		}
		if v != nil {
			count++
		}
	}
	return count, nil
}

// evalSum implements SUM([DISTINCT] expr).
func evalSum(fn *planner.ResolvedFunctionCall, groupRows []row) (any, error) {
	if len(groupRows) == 0 {
		return nil, nil // SUM of empty set is NULL
	}

	var sum float64
	hasValue := false
	seen := make(map[any]struct{})

	for _, r := range groupRows {
		v, err := evalExpr(fn.Args[0], r)
		if err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		if fn.Distinct {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
		}
		f, err := toFloat64(v)
		if err != nil {
			return nil, err
		}
		sum += f
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}
	return coerceResultByKind(sum, fn.ResolvedType()), nil
}

// evalAvg implements AVG([DISTINCT] expr).
func evalAvg(fn *planner.ResolvedFunctionCall, groupRows []row) (any, error) {
	if len(groupRows) == 0 {
		return nil, nil
	}

	var sum float64
	var count int64
	seen := make(map[any]struct{})

	for _, r := range groupRows {
		v, err := evalExpr(fn.Args[0], r)
		if err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		if fn.Distinct {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
		}
		f, err := toFloat64(v)
		if err != nil {
			return nil, err
		}
		sum += f
		count++
	}
	if count == 0 {
		return nil, nil
	}
	return sum / float64(count), nil
}

// evalMinMax implements MIN(expr) and MAX(expr).
func evalMinMax(fn *planner.ResolvedFunctionCall, groupRows []row, isMax bool) (any, error) {
	if len(groupRows) == 0 {
		return nil, nil
	}

	var best any
	for _, r := range groupRows {
		v, err := evalExpr(fn.Args[0], r)
		if err != nil {
			return nil, err
		}
		if v == nil {
			continue
		}
		if best == nil {
			best = v
			continue
		}
		cmp, err := compareValues(v, best)
		if err != nil {
			return nil, err
		}
		if (isMax && cmp > 0) || (!isMax && cmp < 0) {
			best = v
		}
	}
	return best, nil
}

// execSort sorts Input's rows by SortItems.
func (e *Executor) execSort(ctx context.Context, node *planner.SortNode) ([]row, error) {
	rows, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	var sortErr error
	sort.SliceStable(rows, func(i, j int) bool {
		if sortErr != nil {
			return false
		}
		for _, item := range node.Items {
			vi, err := evalExpr(item.Expr, rows[i])
			if err != nil {
				sortErr = err
				return false
			}
			vj, err := evalExpr(item.Expr, rows[j])
			if err != nil {
				sortErr = err
				return false
			}
			// NULLs sort last in ASC, first in DESC (PostgreSQL convention).
			if vi == nil && vj == nil {
				continue
			}
			if vi == nil {
				return item.Direction == ast.OrderDesc
			}
			if vj == nil {
				return item.Direction == ast.OrderAsc
			}
			cmp, err := compareValues(vi, vj)
			if err != nil {
				sortErr = err
				return false
			}
			if cmp == 0 {
				continue
			}
			if item.Direction == ast.OrderDesc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	if sortErr != nil {
		return nil, sortErr
	}

	return rows, nil
}

// execLimit applies OFFSET and LIMIT to Input's rows.
func (e *Executor) execLimit(ctx context.Context, node *planner.LimitNode) ([]row, error) {
	rows, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	// Apply offset.
	if node.Offset > 0 {
		if int(node.Offset) >= len(rows) {
			return nil, nil
		}
		rows = rows[node.Offset:]
	}

	// Apply limit.
	if node.Count > 0 && int(node.Count) < len(rows) {
		rows = rows[:node.Count]
	}

	return rows, nil
}

// execDistinct deduplicates rows.
func (e *Executor) execDistinct(ctx context.Context, node *planner.DistinctNode) ([]row, error) {
	rows, err := e.execRelNode(ctx, node.Input)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var result []row
	for _, r := range rows {
		key := fmt.Sprintf("%v", r.values)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, r)
	}
	return result, nil
}
