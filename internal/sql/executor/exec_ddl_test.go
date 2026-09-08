package executor

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestCreateDatabase(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if !cat.DatabaseExists("mydb") {
		t.Error("database 'mydb' should exist in catalog after creation")
	}
}

func TestCreateDatabaseNoOp(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb", NoOp: true})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
}

func TestUseDatabase(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.UseDatabasePlan{Name: "mydb"})
	if result.SessionUpdate == nil {
		t.Fatal("expected SessionUpdate")
	}
	if result.SessionUpdate.ActiveDatabase != "mydb" {
		t.Errorf("expected active database 'mydb', got %q", result.SessionUpdate.ActiveDatabase)
	}
}

func TestDropDatabase(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "dropme"})
	result := mustExec(t, exec, &planner.DropDatabasePlan{Name: "dropme"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if cat.DatabaseExists("dropme") {
		t.Error("database should not exist after drop")
	}
}

func TestCreateTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})

	schema := testTableMeta()
	result := mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
	})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if _, err := cat.GetTable("testdb", "users"); err != nil {
		t.Errorf("table should exist in catalog: %v", err)
	}
}

func TestCreateTableSnowflake(t *testing.T) {
	exec, cat, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb"})

	schema := &catalog.TableMeta{
		Database:       "mydb",
		Name:           "events",
		HasSnowflakeID: true,
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeBigInt, PrimaryKey: true},
		},
		PrimaryKey: []string{"id"},
	}
	result := mustExec(t, exec, &planner.CreateTablePlan{
		Database: "mydb",
		Table:    "events",
		Schema:   schema,
	})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if _, err := cat.GetTable("mydb", "events"); err != nil {
		t.Fatalf("table should exist in catalog: %v", err)
	}
}

func TestDropTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})
	mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   testTableMeta(),
	})

	result := mustExec(t, exec, &planner.DropTablePlan{Database: "testdb", Table: "users"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if _, err := cat.GetTable("testdb", "users"); err == nil {
		t.Error("table should not exist after drop")
	}
}

func TestAlterTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb"})

	oldSchema := &catalog.TableMeta{
		Database: "mydb",
		Name:     "users",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
		},
		PrimaryKey: []string{"id"},
		Version:    1,
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "mydb", Table: "users", Schema: oldSchema})

	newSchema := &catalog.TableMeta{
		Database: "mydb",
		Name:     "users",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "email", Type: ast.TypeVarchar},
		},
		PrimaryKey: []string{"id"},
		Version:    2,
	}
	result := mustExec(t, exec, &planner.AlterTablePlan{OldSchema: oldSchema, NewSchema: newSchema})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	table, err := cat.GetTable("mydb", "users")
	if err != nil {
		t.Fatalf("table should exist after alter: %v", err)
	}
	if len(table.Columns) != 2 || table.Columns[1].Name != "email" {
		t.Fatalf("unexpected altered schema: %+v", table.Columns)
	}
}

func TestRenameTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb"})

	oldSchema := &catalog.TableMeta{
		Database: "mydb",
		Name:     "old_users",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
		},
		PrimaryKey: []string{"id"},
	}
	mustExec(t, exec, &planner.CreateTablePlan{Database: "mydb", Table: "old_users", Schema: oldSchema})

	newSchema := *oldSchema
	newSchema.Name = "new_users"
	result := mustExec(t, exec, &planner.RenameTablePlan{
		Database: "mydb",
		OldName:  "old_users",
		NewName:  "new_users",
		Schema:   &newSchema,
	})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if _, err := cat.GetTable("mydb", "old_users"); err == nil {
		t.Error("old table should not exist after rename")
	}
	if _, err := cat.GetTable("mydb", "new_users"); err != nil {
		t.Errorf("new table should exist after rename: %v", err)
	}
}
