package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func starCol() *ast.SelectColumn { return &ast.SelectColumn{Star: true} }

func exprCol(e ast.Expression, alias string) *ast.SelectColumn {
	return &ast.SelectColumn{Expr: &ast.SelectExpression{Expr: e}, Alias: alias}
}

func condCol(c ast.Condition, alias string) *ast.SelectColumn {
	return &ast.SelectColumn{Expr: &ast.SelectExpression{Cond: c}, Alias: alias}
}

func fromUsers(alias string) []*ast.TableRef {
	return []*ast.TableRef{tableRef(primary(ident("users"), alias))}
}

// --- basic SELECT: FROM, projection, star -------------------------------

func TestPlanSelect_StarExpansion(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{starCol()}, From: fromUsers("u")})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if len(plan.Columns) != 2 {
		t.Fatalf("expected 2 columns (id, name), got %d: %+v", len(plan.Columns), plan.Columns)
	}
	if _, ok := plan.Root.(*ProjectNode); !ok {
		t.Errorf("expected root *ProjectNode, got %T", plan.Root)
	}
}

func TestPlanSelect_NoFrom_LiteralProjection(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{exprCol(intLit("1"), "")}})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	proj := plan.Root.(*ProjectNode)
	if proj.Input != nil {
		t.Errorf("expected nil Input for a FROM-less SELECT, got %v", proj.Input)
	}
}

func TestPlanSelect_ExplicitAlias(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planSelect(&ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "name"), "n")},
		From:    fromUsers("u"),
	})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if plan.Columns[0].Name != "n" {
		t.Errorf("expected alias %q, got %q", "n", plan.Columns[0].Name)
	}
}

func TestPlanSelect_DefaultAliasFromColumnName(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planSelect(&ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "name"), "")},
		From:    fromUsers("u"),
	})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if plan.Columns[0].Name != "name" {
		t.Errorf("expected default alias %q, got %q", "name", plan.Columns[0].Name)
	}
}

func TestPlanSelect_ConditionAsSelectItem(t *testing.T) {
	// SELECT id = 1 FROM users -- exercises the SelectExpression Cond side.
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	cond := &ast.ComparisonPredicate{Left: qualifiedIdent("u", "id"), Op: utils.TOKEN_EQ, Right: intLit("1")}
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{condCol(cond, "is_first")}, From: fromUsers("u")})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if plan.Columns[0].Type != ast.TypeBoolean {
		t.Errorf("expected TypeBoolean, got %v", plan.Columns[0].Type)
	}
}

func TestPlanSelect_UnknownColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "nope"), "")}, From: fromUsers("u")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownColumn {
		t.Errorf("expected CodeUnknownColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- table.* and db.table.* -------------------------------------------

func TestPlanSelect_QualifiedStar(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	col := &ast.SelectColumn{QualifiedStar: ident("u")}
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{col}, From: fromUsers("u")})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if len(plan.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(plan.Columns))
	}
}

func TestPlanSelect_DBQualifiedStar(t *testing.T) {
	// db.table.* packs into Identifier{Qualifier: db, Name: table} -- a
	// database qualifier, not a table alias, unlike everywhere else.
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	col := &ast.SelectColumn{QualifiedStar: qualifiedIdent("shop", "u")}
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{col}, From: fromUsers("u")})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if len(plan.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(plan.Columns))
	}
}

func TestPlanSelect_DBQualifiedStar_WrongDB_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	col := &ast.SelectColumn{QualifiedStar: qualifiedIdent("archive", "u")}
	_, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{col}, From: fromUsers("u")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownTableBinding {
		t.Errorf("expected CodeUnknownTableBinding, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- JOIN ------------------------------------------------------------

func TestPlanSelect_InnerJoin(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	from := []*ast.TableRef{
		tableRef(
			primary(ident("users"), "u"),
			&ast.JoinClause{
				Type:  ast.JoinInner,
				Right: primary(ident("orders"), "o"),
				On: &ast.ComparisonPredicate{
					Left: qualifiedIdent("u", "id"), Op: utils.TOKEN_EQ, Right: qualifiedIdent("o", "user_id"),
				},
			},
		),
	}
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{starCol()}, From: from})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	proj := plan.Root.(*ProjectNode)
	join, ok := proj.Input.(*JoinNode)
	if !ok {
		t.Fatalf("expected *JoinNode, got %T", proj.Input)
	}
	if join.Type != ast.JoinInner || join.On == nil {
		t.Errorf("unexpected join: type=%v on=%v", join.Type, join.On)
	}
}

func TestPlanSelect_CommaCrossJoin(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	from := []*ast.TableRef{tableRef(primary(ident("users"), "u")), tableRef(primary(ident("orders"), "o"))}
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{starCol()}, From: from})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	proj := plan.Root.(*ProjectNode)
	join, ok := proj.Input.(*JoinNode)
	if !ok || join.Type != ast.JoinCross {
		t.Fatalf("expected implicit *JoinNode{Type: JoinCross}, got %T %v", proj.Input, join)
	}
}

func TestPlanSelect_LeftJoin_Unsupported(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	from := []*ast.TableRef{
		tableRef(primary(ident("users"), "u"), &ast.JoinClause{Type: ast.JoinLeft, Right: primary(ident("orders"), "o")}),
	}
	_, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{starCol()}, From: from})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnsupportedJoinType {
		t.Errorf("expected CodeUnsupportedJoinType, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- WHERE -------------------------------------------------------------

func TestPlanSelect_Where(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{starCol()},
		From:    fromUsers("u"),
		Where:   &ast.WhereClause{Cond: &ast.ComparisonPredicate{Left: qualifiedIdent("u", "id"), Op: utils.TOKEN_EQ, Right: intLit("1")}},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	proj := plan.Root.(*ProjectNode)
	if _, ok := proj.Input.(*FilterNode); !ok {
		t.Errorf("expected *FilterNode under the projection, got %T", proj.Input)
	}
}

func TestPlanSelect_WhereWithoutFrom_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(intLit("1"), "")},
		Where:   &ast.WhereClause{Cond: &ast.ComparisonPredicate{Left: intLit("1"), Op: utils.TOKEN_EQ, Right: intLit("1")}},
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeWhereWithoutFrom {
		t.Errorf("expected CodeWhereWithoutFrom, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanSelect_AggregateInWhere_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	countCall := &ast.FunctionCall{Name: "COUNT", Star: true}
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{starCol()},
		From:    fromUsers("u"),
		Where:   &ast.WhereClause{Cond: &ast.ComparisonPredicate{Left: countCall, Op: utils.TOKEN_GT, Right: intLit("1")}},
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeAggregateInWhere {
		t.Errorf("expected CodeAggregateInWhere, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- GROUP BY / aggregates / HAVING --------------------------------------

func TestPlanSelect_GroupByAggregate(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{
			exprCol(qualifiedIdent("o", "user_id"), ""),
			exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt"),
		},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
		GroupBy: &ast.GroupByClause{Columns: []*ast.Identifier{qualifiedIdent("o", "user_id")}},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	agg, ok := plan.Root.(*AggregateNode)
	if !ok {
		t.Fatalf("expected *AggregateNode, got %T", plan.Root)
	}
	if len(agg.GroupBy) != 1 || len(agg.Aggregates) != 2 {
		t.Errorf("unexpected aggregate node: %+v", agg)
	}
}

func TestPlanSelect_AggregateNoGroupBy_ImplicitSingleGroup(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt")},
		From:    fromUsers("u"),
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	agg := plan.Root.(*AggregateNode)
	if len(agg.GroupBy) != 0 {
		t.Errorf("expected no GROUP BY keys, got %d", len(agg.GroupBy))
	}
}

func TestPlanSelect_NonGroupedColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{
			exprCol(qualifiedIdent("u", "name"), ""),
			exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt"),
		},
		From: fromUsers("u"),
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeMissingGroupBy {
		t.Errorf("expected CodeMissingGroupBy, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanSelect_Having(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{
			exprCol(qualifiedIdent("o", "user_id"), ""),
			exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt"),
		},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
		GroupBy: &ast.GroupByClause{Columns: []*ast.Identifier{qualifiedIdent("o", "user_id")}},
		Having: &ast.HavingClause{Cond: &ast.ComparisonPredicate{
			Left: &ast.FunctionCall{Name: "COUNT", Star: true}, Op: utils.TOKEN_GT, Right: intLit("1"),
		}},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	filter, ok := plan.Root.(*FilterNode)
	if !ok {
		t.Fatalf("expected *FilterNode wrapping the aggregate, got %T", plan.Root)
	}
	if _, ok := filter.Input.(*AggregateNode); !ok {
		t.Errorf("expected *AggregateNode under HAVING's filter, got %T", filter.Input)
	}
}

func TestPlanSelect_HavingWithoutGroupByOrAggregate_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "name"), "")},
		From:    fromUsers("u"),
		Having:  &ast.HavingClause{Cond: &ast.ComparisonPredicate{Left: intLit("1"), Op: utils.TOKEN_EQ, Right: intLit("1")}},
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeMissingGroupBy {
		t.Errorf("expected CodeMissingGroupBy, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanSelect_AggregateWithoutFrom_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	stmt := &ast.SelectStmt{Columns: []*ast.SelectColumn{exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "")}}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeAggregateWithoutFrom {
		t.Errorf("expected CodeAggregateWithoutFrom, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- DISTINCT, ORDER BY, LIMIT --------------------------------------------

func TestPlanSelect_Distinct(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planSelect(&ast.SelectStmt{Columns: []*ast.SelectColumn{starCol()}, From: fromUsers("u"), Distinct: true})
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if _, ok := plan.Root.(*DistinctNode); !ok {
		t.Errorf("expected *DistinctNode, got %T", plan.Root)
	}
}

func TestPlanSelect_OrderByColumn(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{starCol()},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: qualifiedIdent("u", "name"), Direction: ast.OrderAsc}}},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	if _, ok := plan.Root.(*SortNode); !ok {
		t.Errorf("expected *SortNode, got %T", plan.Root)
	}
}

func TestPlanSelect_OrderByAlias(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "name"), "n")},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: ident("n"), Direction: ast.OrderAsc}}},
	}
	_, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("expected ORDER BY to resolve against the SELECT-list alias, got %v", err)
	}
}

func TestPlanSelect_OrderByOrdinal(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "id"), ""), exprCol(qualifiedIdent("u", "name"), "")},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: intLit("2"), Direction: ast.OrderDesc}}},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	sort := plan.Root.(*SortNode)
	if sort.Items[0].Expr.ResolvedType() != ast.TypeVarchar {
		t.Errorf("expected ORDER BY 2 to resolve to the name column, got type %v", sort.Items[0].Expr.ResolvedType())
	}
}

func TestPlanSelect_OrderByOrdinal_OutOfRange_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("u", "id"), "")},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: intLit("5"), Direction: ast.OrderAsc}}},
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeOrdinalOutOfRange {
		t.Errorf("expected CodeOrdinalOutOfRange, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanSelect_Limit(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	offset := 5
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{starCol()},
		From:    fromUsers("u"),
		Limit:   &ast.LimitClause{Count: 10, Offset: &offset},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	limit, ok := plan.Root.(*LimitNode)
	if !ok || limit.Count != 10 || limit.Offset != 5 {
		t.Errorf("unexpected limit node: %+v", limit)
	}
}

// --- pipeline ordering: LIMIT wraps ORDER BY wraps DISTINCT wraps SELECT list --

func TestPlanSelect_FullPipelineOrdering(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns:  []*ast.SelectColumn{starCol()},
		From:     fromUsers("u"),
		Distinct: true,
		OrderBy:  &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: qualifiedIdent("u", "id"), Direction: ast.OrderAsc}}},
		Limit:    &ast.LimitClause{Count: 10},
	}
	plan, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("planSelect: %v", err)
	}
	limit, ok := plan.Root.(*LimitNode)
	if !ok {
		t.Fatalf("expected root *LimitNode, got %T", plan.Root)
	}
	sort, ok := limit.Input.(*SortNode)
	if !ok {
		t.Fatalf("expected *SortNode under LIMIT, got %T", limit.Input)
	}
	distinct, ok := sort.Input.(*DistinctNode)
	if !ok {
		t.Fatalf("expected *DistinctNode under ORDER BY, got %T", sort.Input)
	}
	if _, ok := distinct.Input.(*ProjectNode); !ok {
		t.Fatalf("expected *ProjectNode under DISTINCT, got %T", distinct.Input)
	}
}

// --- ORDER BY aggregate validation ----------------------------------------

func TestPlanSelect_OrderByNonGroupedColumn_InAggregate_Errors(t *testing.T) {
	// SELECT COUNT(*) FROM users ORDER BY users.name — name is not in GROUP BY
	// and is not an aggregate; this must be rejected.
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt")},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: qualifiedIdent("u", "name"), Direction: ast.OrderAsc}}},
	}
	_, err := pc.planSelect(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeMissingGroupBy {
		t.Errorf("expected CodeMissingGroupBy, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanSelect_OrderByGroupedColumn_InAggregate_Succeeds(t *testing.T) {
	// SELECT user_id, COUNT(*) FROM orders GROUP BY user_id ORDER BY user_id
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{
			exprCol(qualifiedIdent("o", "user_id"), ""),
			exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt"),
		},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
		GroupBy: &ast.GroupByClause{Columns: []*ast.Identifier{qualifiedIdent("o", "user_id")}},
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: qualifiedIdent("o", "user_id"), Direction: ast.OrderAsc}}},
	}
	_, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("expected ORDER BY a GROUP BY key to succeed, got %v", err)
	}
}

func TestPlanSelect_OrderByAlias_InAggregate_Succeeds(t *testing.T) {
	// SELECT COUNT(*) AS cnt FROM users ORDER BY cnt — alias bypasses scope validation.
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(&ast.FunctionCall{Name: "COUNT", Star: true}, "cnt")},
		From:    fromUsers("u"),
		OrderBy: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: ident("cnt"), Direction: ast.OrderAsc}}},
	}
	_, err := pc.planSelect(stmt)
	if err != nil {
		t.Fatalf("expected ORDER BY alias to succeed in aggregate query, got %v", err)
	}
}
