package executor

import (
	"context"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestExecInsertSelect(t *testing.T) {
	exec, _, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})

	// Create source table
	srcMeta := &catalog.TableMeta{
		Database: "testdb",
		Name:     "src",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "val", Type: ast.TypeVarchar},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "testdb", Table: "src", Schema: srcMeta})

	// Create dest table with SnowflakeID to also test sequence reading/writing
	dstMeta := &catalog.TableMeta{
		Database:       "testdb",
		Name:           "dst",
		HasSnowflakeID: true,
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeBigInt, PrimaryKey: true},
			{Name: "val", Type: ast.TypeVarchar},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "testdb", Table: "dst", Schema: dstMeta})

	// Insert into src
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "src",
		Schema:   srcMeta,
		Columns: []planner.ResolvedColumn{
			{Database: "testdb", Table: "src", Name: "id", Index: 0, Type: ast.TypeInt},
			{Database: "testdb", Table: "src", Name: "val", Index: 1, Type: ast.TypeVarchar},
		},
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a"},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b"},
			},
		},
	})

	// Execute INSERT INTO dst SELECT * FROM src
	// Since dst has Snowflake ID, select only the source value column.
	srcTable := &planner.ResolvedTable{
		Database: "testdb",
		Table:    "src",
		Binding:  "src",
		Schema:   srcMeta,
	}
	srcPlan := &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: srcTable},
			Items: []planner.ProjectItem{{
				Expr: &planner.ResolvedColumnRef{Column: planner.ResolvedColumn{
					Database: "testdb", Table: "src", Name: "val", Index: 1, Type: ast.TypeVarchar,
				}},
				Alias: "val",
			}},
		},
		Columns: []planner.OutputColumn{{Name: "val", Type: ast.TypeVarchar}},
	}

	result := mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "dst",
		Schema:   dstMeta,
		Columns: []planner.ResolvedColumn{
			{Database: "testdb", Table: "dst", Name: "val", Index: 1, Type: ast.TypeVarchar},
		},
		Source: srcPlan,
	})

	if result.Type != ResultDML {
		t.Errorf("expected ResultDML, got %d", result.Type)
	}
	if result.RowsAffected != 2 {
		t.Errorf("expected 2 rows affected, got %d", result.RowsAffected)
	}

	// Verify dst table contents
	scanPlan := &planner.QueryPlan{
		Root: &planner.ScanNode{Table: &planner.ResolvedTable{
			Database: "testdb",
			Table:    "dst",
			Binding:  "dst",
			Schema:   dstMeta,
		}},
	}
	res, err := exec.execQuery(context.Background(), scanPlan)
	if err != nil {
		t.Fatalf("scan dst failed: %v", err)
	}

	if len(res.Rows) != 2 {
		t.Errorf("expected 2 rows in dst, got %d", len(res.Rows))
	}
	if res.Rows[0][1] != "a" || res.Rows[1][1] != "b" {
		t.Errorf("unexpected rows in dst: %v", res.Rows)
	}
}

func TestReadSequence(t *testing.T) {
	exec, _, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})

	meta := &catalog.TableMeta{
		Database:       "testdb",
		Name:           "seqtest",
		HasSnowflakeID: true,
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeBigInt, PrimaryKey: true},
			{Name: "val", Type: ast.TypeInt},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "testdb", Table: "seqtest", Schema: meta})

	// readSequence should return 0 initially (set by create table)
	seq, err := exec.readSequence(context.Background(), "testdb", "seqtest")
	if err != nil {
		t.Fatalf("readSequence failed: %v", err)
	}
	if seq != 0 {
		t.Errorf("expected sequence 0, got %d", seq)
	}

	// Insert a row, should increment sequence to 1
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "seqtest",
		Schema:   meta,
		Columns: []planner.ResolvedColumn{
			{Database: "testdb", Table: "seqtest", Name: "val", Index: 1, Type: ast.TypeInt},
		},
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 100},
			},
		},
	})

	seq, err = exec.readSequence(context.Background(), "testdb", "seqtest")
	if err != nil {
		t.Fatalf("readSequence failed: %v", err)
	}
	if seq != 1 {
		t.Errorf("expected sequence 1, got %d", seq)
	}
}

func TestInsertAndSelect(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// INSERT one row.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	// SELECT * FROM users (scan all).
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb",
		Table:    "users",
		Binding:  "users",
		Schema:   schema,
	}
	// We can't use the private constructor, so we'll test via QueryPlan.
	// Build a simple scan+project query plan.
	scanNode := &planner.ScanNode{Table: resolvedTable}
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: scanNode,
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[2]}, Alias: "email"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[3]}, Alias: "active"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean},
		},
	})

	if result.Type != ResultQuery {
		t.Fatalf("expected ResultQuery, got %d", result.Type)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row[0] != int32(1) {
		t.Errorf("id: expected int32(1), got %v (%T)", row[0], row[0])
	}
	if row[1] != "Alice" {
		t.Errorf("name: expected 'Alice', got %v", row[1])
	}
	if row[2] != "alice@test.com" {
		t.Errorf("email: expected 'alice@test.com', got %v", row[2])
	}
	if row[3] != true {
		t.Errorf("active: expected true, got %v", row[3])
	}
}

func TestInsertMultipleAndSelectWithFilter(t *testing.T) {
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
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "bob@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "charlie@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT id, name FROM users WHERE active = TRUE
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.FilterNode{
				Input: &planner.ScanNode{Table: resolvedTable},
				Cond: &planner.ResolvedComparison{
					Left:  &planner.ResolvedColumnRef{Column: cols[3]},
					Op:    planner.OpEq,
					Right: &planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
				},
			},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 active users, got %d", len(result.Rows))
	}
	// Alice (id=1) and Charlie (id=3) should be the active users.
	if result.Rows[0][0] != int32(1) {
		t.Errorf("first active user should be id=1, got %v", result.Rows[0][0])
	}
	if result.Rows[1][0] != int32(3) {
		t.Errorf("second active user should be id=3, got %v", result.Rows[1][0])
	}
}

func TestUpdate(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert a row.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	// UPDATE users SET name = 'Alice Updated' WHERE id = 1
	updateResult := mustExec(t, exec, &planner.UpdatePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Assignments: []planner.Assignment{
			{
				Column: cols[1],
				Value:  &planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice Updated"},
			},
		},
		Where: &planner.ResolvedComparison{
			Left:  &planner.ResolvedColumnRef{Column: cols[0]},
			Op:    planner.OpEq,
			Right: &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
		},
	})

	if updateResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", updateResult.RowsAffected)
	}

	// Verify the update by selecting.
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}
	selectResult := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{{Name: "name", Type: ast.TypeVarchar}},
	})
	if len(selectResult.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(selectResult.Rows))
	}
	if selectResult.Rows[0][0] != "Alice Updated" {
		t.Errorf("expected 'Alice Updated', got %v", selectResult.Rows[0][0])
	}
}

func TestDelete(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 2 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "bob@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
		},
	})

	// DELETE FROM users WHERE id = 1
	deleteResult := mustExec(t, exec, &planner.DeletePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Where: &planner.ResolvedComparison{
			Left:  &planner.ResolvedColumnRef{Column: cols[0]},
			Op:    planner.OpEq,
			Right: &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
		},
	})

	if deleteResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", deleteResult.RowsAffected)
	}

	// Verify only Bob remains.
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}
	selectResult := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	})
	if len(selectResult.Rows) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(selectResult.Rows))
	}
	if selectResult.Rows[0][0] != int32(2) {
		t.Errorf("remaining user should be id=2, got %v", selectResult.Rows[0][0])
	}
}

func TestDuplicateKeyError(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	insertPlan := &planner.InsertPlan{
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
		},
	}

	mustExec(t, exec, insertPlan)

	// Insert again with same PK should fail.
	_, err := exec.Execute(context.Background(), insertPlan)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestNotNullViolation(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Try to insert NULL into NOT NULL column 'active'.
	_, err := exec.Execute(context.Background(), &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedNullLiteral{}, // NULL for NOT NULL column
			},
		},
	})
	if err == nil {
		t.Fatal("expected NOT NULL violation error")
	}
}
