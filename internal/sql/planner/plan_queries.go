package planner

import (
	"strconv"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// planSelect plans a full SELECT statement, following SQL's logical
// evaluation order rather than its written order: FROM, WHERE, GROUP BY,
// HAVING, the SELECT list, DISTINCT, ORDER BY, then LIMIT. The return type
// is kept concrete (*QueryPlan, not Plan) so planInsert can read
// Source.Columns directly for INSERT ... SELECT — planStatement widens it
// to Plan when dispatching.
func (pc *planContext) planSelect(stmt *ast.SelectStmt) (*QueryPlan, error) {
	root, scope, err := pc.buildFromPlan(stmt.From)
	if err != nil {
		return nil, err
	}

	if stmt.Where != nil {
		if root == nil {
			return nil, pc.errorf(stmt.Where.Span(), CodeWhereWithoutFrom, "WHERE requires a FROM clause")
		}
		cond, err := pc.resolveCond(scope, stmt.Where.Cond)
		if err != nil {
			return nil, err
		}
		if condHasAggregate(cond) {
			return nil, pc.errorf(stmt.Where.Span(), CodeAggregateInWhere, "aggregate functions are not allowed in WHERE")
		}
		root = &FilterNode{Input: root, Cond: cond}
	}

	items, err := pc.resolveSelectList(scope, stmt.Columns)
	if err != nil {
		return nil, err
	}

	var havingCond ResolvedCond
	var havingSpan diagnostic.Span
	if stmt.Having != nil {
		havingSpan = stmt.Having.Span()
		havingCond, err = pc.resolveCond(scope, stmt.Having.Cond)
		if err != nil {
			return nil, err
		}
	}

	isAggregate := stmt.GroupBy != nil || anyItemHasAggregate(items) || condHasAggregate(havingCond)

	switch {
	case isAggregate && root == nil:
		return nil, pc.errorf(stmt.Span(), CodeAggregateWithoutFrom, "aggregate queries require a FROM clause")
	case isAggregate:
		root, err = pc.buildAggregatePlan(root, scope, stmt, items, havingCond, havingSpan)
		if err != nil {
			return nil, err
		}
	case stmt.Having != nil:
		return nil, pc.errorf(havingSpan, CodeMissingGroupBy, "HAVING requires GROUP BY or an aggregate function")
	default:
		root = &ProjectNode{Input: root, Items: items}
	}

	if stmt.Distinct {
		root = &DistinctNode{Input: root}
	}

	if stmt.OrderBy != nil {
		root, err = pc.buildSortPlan(root, scope, items, stmt.OrderBy)
		if err != nil {
			return nil, err
		}
	}

	if stmt.Limit != nil {
		offset := 0
		if stmt.Limit.Offset != nil {
			offset = *stmt.Limit.Offset
		}
		root = &LimitNode{Input: root, Count: stmt.Limit.Count, Offset: offset}
	}

	return &QueryPlan{Root: root, Columns: outputColumns(items)}, nil
}

// buildFromPlan resolves every table reference in a FROM clause, building
// both the RelNode scan/join tree that will actually read rows at
// execution time and the Scope used to resolve column identifiers against
// it. root is nil when refs is empty (a FROM-less SELECT, e.g. SELECT
// 1+1). Comma-separated top-level references are chained together with an
// implicit CROSS JOIN, matching how ast.TableRef's own Joins list is
// folded within a single reference.
//
// Every reference is still walked even after one fails, so a FROM clause
// with several unrelated problems reports all of them in one pass — but
// the first failure is returned as err, since resolving WHERE or the
// SELECT list against a FROM tree that's known to be broken would just
// produce further, less meaningful diagnostics on top of it.
func (pc *planContext) buildFromPlan(refs []*ast.TableRef) (root RelNode, scope *Scope, err error) {
	scope = newScope()
	for _, ref := range refs {
		next, refErr := pc.walkTableRefPlan(ref, scope)
		if refErr != nil && err == nil {
			err = refErr
		}
		if next == nil {
			continue
		}
		if root == nil {
			root = next
			continue
		}
		root = &JoinNode{Left: root, Right: next, Type: ast.JoinCross}
	}
	return root, scope, err
}

// buildScope builds only the Scope for a FROM clause, discarding the
// RelNode tree and any error — used by tests and helpers that only need
// name resolution against a set of table references, not a runnable
// relational plan.
func (pc *planContext) buildScope(refs []*ast.TableRef) *Scope {
	_, scope, _ := pc.buildFromPlan(refs)
	return scope
}

// walkTableRefPlan is buildFromPlan's per-TableRef worker: it builds the
// scan/join subtree for one FROM-clause entry — recursing through
// parentheses, then folding its Joins left-to-right — while registering
// every table it touches in scope.
func (pc *planContext) walkTableRefPlan(ref *ast.TableRef, scope *Scope) (RelNode, error) {
	var root RelNode
	var err error
	if ref.Paren != nil {
		root, err = pc.walkTableRefPlan(ref.Paren, scope)
	} else {
		root, err = pc.addTablePrimaryPlan(ref.Primary, scope)
	}
	if root == nil {
		return nil, err
	}

	for _, join := range ref.Joins {
		switch join.Type {
		case ast.JoinLeft, ast.JoinRight, ast.JoinFull:
			joinErr := pc.errorf(
				join.Span(), CodeUnsupportedJoinType,
				"join type not supported yet: only INNER and CROSS JOIN are implemented",
			)
			if err == nil {
				err = joinErr
			}
			continue
		}

		right, rightErr := pc.addTablePrimaryPlan(join.Right, scope)
		if rightErr != nil && err == nil {
			err = rightErr
		}
		if right == nil {
			continue
		}

		var on ResolvedCond
		if join.On != nil {
			cond, condErr := pc.resolveCond(scope, join.On)
			if condErr != nil {
				if err == nil {
					err = condErr
				}
				continue
			}
			if condHasAggregate(cond) {
				joinErr := pc.errorf(
					join.On.Span(),
					CodeAggregateInJoin,
					"aggregate functions are not allowed in JOIN conditions",
				)
				if err == nil {
					err = joinErr
				}
				continue
			}
			on = cond
		}
		root = &JoinNode{Left: root, Right: right, Type: join.Type, On: on}
	}
	return root, err
}

// addTablePrimaryPlan resolves a single named table, registers it in scope
// under its alias (or its own name), and returns the ScanNode that reads
// it.
func (pc *planContext) addTablePrimaryPlan(primary *ast.TablePrimary, scope *Scope) (RelNode, error) {
	meta, _, err := pc.resolveTableIdentifier(primary.Name)
	if err != nil {
		return nil, err
	}

	binding := primary.Alias
	if binding == "" {
		binding = primary.Name.Name
	}

	rt := newResolvedTable(meta, binding)
	if err := scope.addTable(rt); err != nil {
		dupErr := pc.errorf(
			primary.Span(), CodeDuplicateTableBinding,
			"table or alias %q is already used in this FROM clause", binding,
		)
		return nil, dupErr
	}
	return &ScanNode{Table: rt}, nil
}

// resolveSelectList resolves a SELECT statement's column list against
// scope, expanding * and table.* into every visible column.
func (pc *planContext) resolveSelectList(scope *Scope, cols []*ast.SelectColumn) ([]ProjectItem, error) {
	var items []ProjectItem
	var firstErr error

	for _, col := range cols {
		switch {
		case col.Star:
			expanded := expandStar(scope.Tables(), col.Span())
			if len(expanded) == 0 {
				err := pc.errorf(col.Span(), CodeEmptyStarExpansion, "SELECT * matched no columns: no tables in scope")
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			items = append(items, expanded...)

		case col.QualifiedStar != nil:
			// table.* uses Identifier{Name: table}; db.table.* uses
			// Identifier{Qualifier: db, Name: table} — Qualifier here is a
			// database name, not a table alias, unlike every other use of
			// Identifier.Qualifier in this grammar. See parseSelectColumn.
			table, ok := scope.byBinding[col.QualifiedStar.Name]
			if !ok || (col.QualifiedStar.Qualifier != "" && table.Database != col.QualifiedStar.Qualifier) {
				err := pc.errorf(col.QualifiedStar.Span(), CodeUnknownTableBinding, "unknown table %q", col.QualifiedStar.Name)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			for _, rc := range table.Columns() {
				items = append(items, ProjectItem{Expr: &ResolvedColumnRef{Column: rc}, Alias: rc.Name, Span: col.Span()})
			}

		default:
			expr, err := pc.resolveSelectExpression(scope, col.Expr)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			alias := col.Alias
			if alias == "" {
				alias = defaultAlias(expr)
			}
			items = append(items, ProjectItem{Expr: expr, Alias: alias, Span: col.Span()})
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return items, nil
}

func expandStar(tables []*ResolvedTable, span diagnostic.Span) []ProjectItem {
	var items []ProjectItem
	for _, table := range tables {
		for _, rc := range table.Columns() {
			items = append(items, ProjectItem{Expr: &ResolvedColumnRef{Column: rc}, Alias: rc.Name, Span: span})
		}
	}
	return items
}

// defaultAlias picks a display name for a SELECT-list item with no
// explicit AS alias. A bare column reference uses its own column name — a
// function call uses the function's name (COUNT, SUM, ...), matching what
// most engines default to. Anything else (a computed expression, a
// literal) is left blank; the executor or client is expected to synthesize
// a positional name.
func defaultAlias(expr ResolvedExpr) string {
	switch e := expr.(type) {
	case *ResolvedColumnRef:
		return e.Column.Name
	case *ResolvedFunctionCall:
		return e.Name
	default:
		return ""
	}
}

func outputColumns(items []ProjectItem) []OutputColumn {
	cols := make([]OutputColumn, len(items))
	for i, item := range items {
		cols[i] = OutputColumn{Name: item.Alias, Type: item.Expr.ResolvedType()}
	}
	return cols
}

// buildAggregatePlan resolves GROUP BY's key list, validates that every
// SELECT-list item and the HAVING condition (if any) are legal under that
// grouping (see aggregate.go), and wraps root in an AggregateNode and, if
// HAVING was present, a FilterNode on top of it.
func (pc *planContext) buildAggregatePlan(
	root RelNode, scope *Scope, stmt *ast.SelectStmt, items []ProjectItem, having ResolvedCond, havingSpan diagnostic.Span,
) (RelNode, error) {
	var groupExprs []ResolvedExpr
	var groupCols []*ResolvedColumn

	if stmt.GroupBy != nil {
		for _, id := range stmt.GroupBy.Columns {
			col, err := pc.resolveColumn(scope, id)
			if err != nil {
				return nil, err
			}
			groupExprs = append(groupExprs, &ResolvedColumnRef{Column: col})
			groupCols = append(groupCols, col)
		}
	}

	for _, item := range items {
		if err := pc.validateGroupedExpr(item.Expr, groupCols, item.Span); err != nil {
			return nil, err
		}
	}
	if having != nil {
		if err := pc.validateGroupedCond(having, groupCols, havingSpan); err != nil {
			return nil, err
		}
	}

	var result RelNode = &AggregateNode{Input: root, GroupBy: groupExprs, Aggregates: items}
	if having != nil {
		result = &FilterNode{Input: result, Cond: having}
	}
	return result, nil
}

// buildSortPlan resolves an ORDER BY clause.
func (pc *planContext) buildSortPlan(root RelNode, scope *Scope, items []ProjectItem, ob *ast.OrderByClause) (RelNode, error) {
	sortItems := make([]SortItem, 0, len(ob.Items))
	var firstErr error

	for _, item := range ob.Items {
		expr, err := pc.resolveOrderByExpr(scope, items, item.Expr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sortItems = append(sortItems, SortItem{Expr: expr, Direction: item.Direction})
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return &SortNode{Input: root, Items: sortItems}, nil
}

// resolveOrderByExpr resolves one ORDER BY key by trying, in order: an
// ordinal position into the SELECT list (a bare integer literal, e.g.
// ORDER BY 1), a SELECT-list alias (ORDER BY some_alias), and finally a
// normal expression resolved against scope (ORDER BY some_column) — the
// three common ORDER BY forms.
//
// Known gap: for an aggregate query, the scope fallback lets ORDER BY
// resolve any column in the original FROM tables, not just GROUP BY keys
// and aggregate results — unlike the SELECT list and HAVING, this path
// isn't run through validateGroupedExpr. Adding that is possible but was
// left out here to keep this phase's scope bounded.
func (pc *planContext) resolveOrderByExpr(scope *Scope, items []ProjectItem, expr ast.Expression) (ResolvedExpr, error) {
	if lit, ok := expr.(*ast.IntegerLiteral); ok {
		n, convErr := strconv.Atoi(lit.Value)
		if convErr == nil && n >= 1 && n <= len(items) {
			return items[n-1].Expr, nil
		}
		return nil, pc.errorf(
			lit.Span(), CodeOrdinalOutOfRange,
			"ORDER BY position %s is out of range (SELECT list has %d columns)", lit.Value, len(items),
		)
	}

	if id, ok := expr.(*ast.Identifier); ok && id.Qualifier == "" {
		for _, item := range items {
			if item.Alias != "" && item.Alias == id.Name {
				return item.Expr, nil
			}
		}
	}

	return pc.resolveExpr(scope, expr)
}
