package executor

import (
	"context"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestExecJoin(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})

	t1Meta := &catalog.TableMeta{
		Database: "testdb",
		Name:     "t1",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "val1", Type: ast.TypeInt},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "testdb", Table: "t1", Schema: t1Meta})

	t2Meta := &catalog.TableMeta{
		Database: "testdb",
		Name:     "t2",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "t1_id", Type: ast.TypeInt},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "testdb", Table: "t2", Schema: t2Meta})

	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb", Table: "t1", Schema: t1Meta,
		Columns: []planner.ResolvedColumn{
			{Database: "testdb", Table: "t1", Name: "id", Index: 0, Type: ast.TypeInt},
			{Database: "testdb", Table: "t1", Name: "val1", Index: 1, Type: ast.TypeInt},
		},
		Rows: [][]planner.ResolvedExpr{
			{&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1}, &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 100}},
			{&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2}, &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 200}},
		},
	})

	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb", Table: "t2", Schema: t2Meta,
		Columns: []planner.ResolvedColumn{
			{Database: "testdb", Table: "t2", Name: "id", Index: 0, Type: ast.TypeInt},
			{Database: "testdb", Table: "t2", Name: "t1_id", Index: 1, Type: ast.TypeInt},
		},
		Rows: [][]planner.ResolvedExpr{
			{&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1}, &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1}},
			{&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2}, &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1}},
			{&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3}, &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2}},
		},
	})

	// Join on t1.id = t2.t1_id
	joinPlan := &planner.JoinNode{
		Left: &planner.ScanNode{
			Table: &planner.ResolvedTable{Database: "testdb", Table: "t1", Binding: "t1", Schema: t1Meta},
		},
		Right: &planner.ScanNode{
			Table: &planner.ResolvedTable{Database: "testdb", Table: "t2", Binding: "t2", Schema: t2Meta},
		},
		On: &planner.ResolvedComparison{
			Op: planner.OpEq,
			// index 0 is t1.id, index 3 is t2.t1_id
			Left:  &planner.ResolvedColumnRef{Column: planner.ResolvedColumn{Index: 0, Type: ast.TypeInt}},
			Right: &planner.ResolvedColumnRef{Column: planner.ResolvedColumn{Index: 3, Type: ast.TypeInt}},
		},
	}

	rows, err := exec.execJoin(context.Background(), joinPlan)
	if err != nil {
		t.Fatalf("execJoin failed: %v", err)
	}

	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
}

func TestAggregates(t *testing.T) {
	groupRows := []row{
		{values: []any{int32(10), int32(5)}},
		{values: []any{int32(20), int32(5)}},
		{values: []any{int32(30), int32(5)}},
		{values: []any{nil, int32(5)}},  // NULL
		{values: []any{int32(20), nil}}, // duplicate for 20
	}

	// Helper to resolve an aggregate function
	evalFunc := func(name string, argIdx int, distinct bool) (any, error) {
		var args []planner.ResolvedExpr
		if argIdx >= 0 {
			args = append(args, &planner.ResolvedColumnRef{
				Column: planner.ResolvedColumn{Index: argIdx, Type: ast.TypeInt},
			})
		}
		fn := &planner.ResolvedFunctionCall{
			Name:     name,
			Args:     args,
			Distinct: distinct,
			Star:     argIdx < 0, // e.g. COUNT(*)
		}
		switch name {
		case "SUM", "AVG", "MIN", "MAX":
			fn.Type = ast.TypeInt
		case "COUNT":
			fn.Type = ast.TypeBigInt
		}
		return evalAggregateFunc(fn, groupRows)
	}

	// Test COUNT(*)
	res, err := evalFunc("COUNT", -1, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int64) != 5 {
		t.Errorf("COUNT(*) = %v, want 5", res)
	}

	// Test COUNT(col)
	res, err = evalFunc("COUNT", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int64) != 4 {
		t.Errorf("COUNT(col) = %v, want 4", res)
	}

	// Test COUNT(DISTINCT col)
	res, err = evalFunc("COUNT", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int64) != 3 {
		t.Errorf("COUNT(DISTINCT col) = %v, want 3", res)
	}

	// Test SUM(col)
	res, err = evalFunc("SUM", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != 80 { // 10+20+30+20
		t.Errorf("SUM(col) = %v, want 80", res)
	}

	// Test SUM(DISTINCT col)
	res, err = evalFunc("SUM", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != 60 { // 10+20+30
		t.Errorf("SUM(DISTINCT col) = %v, want 60", res)
	}

	// Test AVG(col)
	res, err = evalFunc("AVG", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(float64) != 20.0 { // 80 / 4
		t.Errorf("AVG(col) = %v, want 20", res)
	}

	// Test MIN(col)
	res, err = evalFunc("MIN", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != 10 {
		t.Errorf("MIN(col) = %v, want 10", res)
	}

	// Test MAX(col)
	res, err = evalFunc("MAX", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != 30 {
		t.Errorf("MAX(col) = %v, want 30", res)
	}
}

func TestEvalAggregateExpr(t *testing.T) {
	groupRows := []row{
		{values: []any{int32(10)}},
		{values: []any{int32(20)}},
	}

	// SUM(col0)
	sumFunc := &planner.ResolvedFunctionCall{
		ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
		Name:             "SUM",
		Args: []planner.ResolvedExpr{
			&planner.ResolvedColumnRef{Column: planner.ResolvedColumn{Index: 0, Type: ast.TypeInt}},
		},
	}

	// SUM(col0) + 5
	expr := &planner.ResolvedBinaryExpr{
		Op:    planner.OpAdd,
		Left:  sumFunc,
		Right: &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 5},
	}

	res, err := evalAggregateExpr(expr, groupRows)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != 35 { // 30 + 5
		t.Errorf("evalAggregateExpr = %v, want 35", res)
	}

	// -SUM(col0)
	negExpr := &planner.ResolvedUnaryExpr{
		ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
		Op:               planner.OpSub,
		Operand:          sumFunc,
	}
	res, err = evalAggregateExpr(negExpr, groupRows)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int32) != -30 {
		t.Errorf("evalAggregateExpr = %v, want -30", res)
	}
}

func TestEmptyAggregates(t *testing.T) {
	// Aggregate over empty group
	var emptyRows []row

	fn := &planner.ResolvedFunctionCall{
		ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
		Name:             "SUM",
		Args: []planner.ResolvedExpr{
			&planner.ResolvedColumnRef{Column: planner.ResolvedColumn{Index: 0, Type: ast.TypeInt}},
		},
	}
	res, err := evalAggregateFunc(fn, emptyRows)
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("SUM(empty) = %v, want nil", res)
	}

	fnCount := &planner.ResolvedFunctionCall{
		ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBigInt},
		Name:             "COUNT",
		Star:             true,
	}
	res, err = evalAggregateFunc(fnCount, emptyRows)
	if err != nil {
		t.Fatal(err)
	}
	if res.(int64) != 0 {
		t.Errorf("COUNT(empty) = %v, want 0", res)
	}
}

func TestSelectOrderByAndLimit(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 3 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT id, name FROM users ORDER BY name ASC LIMIT 2
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.LimitNode{
			Input: &planner.SortNode{
				Input: &planner.ProjectNode{
					Input: &planner.ScanNode{Table: resolvedTable},
					Items: []planner.ProjectItem{
						{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
						{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
					},
				},
				Items: []planner.SortItem{
					{Expr: &planner.ResolvedColumnRef{Column: planner.ResolvedColumn{
						Database: "testdb", Table: "users", Name: "name", Index: 1, Type: ast.TypeVarchar,
					}}, Direction: ast.OrderAsc},
				},
			},
			Count: 2,
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	// After sorting by name ASC: Alice, Bob, Charlie → LIMIT 2 → Alice, Bob.
	if result.Rows[0][1] != "Alice" {
		t.Errorf("first row name: expected 'Alice', got %v", result.Rows[0][1])
	}
	if result.Rows[1][1] != "Bob" {
		t.Errorf("second row name: expected 'Bob', got %v", result.Rows[1][1])
	}
}

func TestSelectAggregate(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 3 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT COUNT(*) FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.AggregateNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Aggregates: []planner.ProjectItem{
				{
					Expr: &planner.ResolvedFunctionCall{
						ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBigInt},
						Name:             "COUNT",
						Star:             true,
					},
					Alias: "count",
				},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "count", Type: ast.TypeBigInt},
		},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != int64(3) {
		t.Errorf("COUNT(*) expected 3, got %v (%T)", result.Rows[0][0], result.Rows[0][0])
	}
}

func TestSelectDistinct(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert rows with duplicate active values.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT DISTINCT active FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.DistinctNode{
			Input: &planner.ProjectNode{
				Input: &planner.ScanNode{Table: resolvedTable},
				Items: []planner.ProjectItem{
					{Expr: &planner.ResolvedColumnRef{Column: cols[3]}, Alias: "active"},
				},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "active", Type: ast.TypeBoolean},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 distinct values, got %d", len(result.Rows))
	}
}

func TestSelectFromLessQuery(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	// SELECT 1 + 2 (no FROM clause).
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: nil, // FROM-less → nil input → single implicit row
			Items: []planner.ProjectItem{
				{
					Expr: &planner.ResolvedBinaryExpr{
						ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
						Left:             &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
						Op:               planner.OpAdd,
						Right:            &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
					},
					Alias: "result",
				},
			},
		},
		Columns: []planner.OutputColumn{{Name: "result", Type: ast.TypeInt}},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != int32(3) {
		t.Errorf("expected int32(3), got %v (%T)", result.Rows[0][0], result.Rows[0][0])
	}
}

func TestNullHandling(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert with NULL name and email.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedNullLiteral{},
				&planner.ResolvedNullLiteral{},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT name IS NULL FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{{Name: "name", Type: ast.TypeVarchar}},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != nil {
		t.Errorf("expected nil (NULL), got %v", result.Rows[0][0])
	}
}
