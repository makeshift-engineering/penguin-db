package planner

import (
	"context"
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func colDef(name string, kind ast.DataTypeKind, constraints ...ast.Clause) *ast.ColumnDef {
	return &ast.ColumnDef{Name: name, Type: &ast.DataType{Kind: kind}, Constraints: constraints}
}

func varcharDef(name string, length int, constraints ...ast.Clause) *ast.ColumnDef {
	l := length
	return &ast.ColumnDef{Name: name, Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: &l}, Constraints: constraints}
}

func defaultConstraint(v ast.Expression, negative bool) *ast.DefaultConstraint {
	return &ast.DefaultConstraint{Value: &ast.SignedLiteral{Value: v, Negative: negative}}
}

func TestPlanCreateDatabase_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	plan, err := pc.planCreateDatabase(&ast.CreateDatabaseStmt{Name: "newdb"})
	if err != nil {
		t.Fatalf("planCreateDatabase: %v", err)
	}
	if p := plan.(*CreateDatabasePlan); p.Name != "newdb" || p.NoOp {
		t.Errorf("unexpected plan: %+v", p)
	}
}

func TestPlanCreateDatabase_AlreadyExists(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.planCreateDatabase(&ast.CreateDatabaseStmt{Name: "shop"})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDatabaseExists {
		t.Errorf("expected CodeDatabaseExists, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateDatabase_IfNotExists_NoOp(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	plan, err := pc.planCreateDatabase(&ast.CreateDatabaseStmt{Name: "shop", IfNotExists: true})
	if err != nil {
		t.Fatalf("planCreateDatabase: %v", err)
	}
	if !plan.(*CreateDatabasePlan).NoOp {
		t.Error("expected NoOp=true")
	}
}

func TestPlanUseDatabase_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.planUseDatabase(&ast.UseDatabaseStmt{Name: "shop"})
	if err != nil {
		t.Fatalf("planUseDatabase: %v", err)
	}
}

func TestPlanUseDatabase_Unknown(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.planUseDatabase(&ast.UseDatabaseStmt{Name: "nope"})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownDatabase {
		t.Errorf("expected CodeUnknownDatabase, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanDropDatabase_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	plan, err := pc.planDropDatabase(&ast.DropDatabaseStmt{Name: "shop"})
	if err != nil {
		t.Fatalf("planDropDatabase: %v", err)
	}
	if p := plan.(*DropDatabasePlan); len(p.Tables) != 2 {
		t.Errorf("expected 2 tables (users, orders), got %d", len(p.Tables))
	}
}

func TestPlanDropDatabase_Unknown(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.planDropDatabase(&ast.DropDatabaseStmt{Name: "nope"})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownDatabase {
		t.Errorf("expected CodeUnknownDatabase, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanDropDatabase_IfExists_NoOp(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	plan, err := pc.planDropDatabase(&ast.DropDatabaseStmt{Name: "nope", IfExists: true})
	if err != nil {
		t.Fatalf("planDropDatabase: %v", err)
	}
	if !plan.(*DropDatabasePlan).NoOp {
		t.Error("expected NoOp=true")
	}
}

func TestPlanCreateTable_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("products"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, &ast.PrimaryKeyConstraint{}),
			varcharDef("name", 100, &ast.NotNullConstraint{}),
			colDef("price", ast.TypeDouble, defaultConstraint(floatLit("0"), false)),
		},
	}
	plan, err := pc.planCreateTable(stmt)
	if err != nil {
		t.Fatalf("planCreateTable: %v", err)
	}
	schema := plan.(*CreateTablePlan).Schema
	if len(schema.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(schema.Columns))
	}
	if len(schema.PrimaryKey) != 1 || schema.PrimaryKey[0] != "id" {
		t.Errorf("expected primary key [id], got %v", schema.PrimaryKey)
	}
	if schema.HasSnowflakeID {
		t.Error("HasSnowflakeID should be false when an explicit PK is declared")
	}
	if schema.Columns[2].DefaultValue == nil || *schema.Columns[2].DefaultValue != "0" {
		t.Errorf("expected price default \"0\", got %+v", schema.Columns[2].DefaultValue)
	}
}

func TestPlanCreateTable_NoExplicitPK_UsesSnowflakeID(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table:   ident("events"),
		Columns: []*ast.ColumnDef{colDef("payload", ast.TypeText)},
	}
	plan, err := pc.planCreateTable(stmt)
	if err != nil {
		t.Fatalf("planCreateTable: %v", err)
	}
	if !plan.(*CreateTablePlan).Schema.HasSnowflakeID {
		t.Error("expected HasSnowflakeID=true with no explicit PK")
	}
}

func TestPlanCreateTable_DuplicateColumn(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table:   ident("dupes"),
		Columns: []*ast.ColumnDef{colDef("id", ast.TypeInt), colDef("id", ast.TypeInt)},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDuplicateColumn {
		t.Errorf("expected CodeDuplicateColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_AlreadyExists(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{Table: ident("users"), Columns: []*ast.ColumnDef{colDef("id", ast.TypeInt)}}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTableExists {
		t.Errorf("expected CodeTableExists, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_IfNotExists_NoOp(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table:       ident("users"),
		IfNotExists: true,
		Columns:     []*ast.ColumnDef{colDef("id", ast.TypeInt)},
	}
	plan, err := pc.planCreateTable(stmt)
	if err != nil {
		t.Fatalf("planCreateTable: %v", err)
	}
	if !plan.(*CreateTablePlan).NoOp {
		t.Error("expected NoOp=true")
	}
}

func TestPlanCreateTable_UnknownDatabase(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	stmt := &ast.CreateTableStmt{
		Table:   qualifiedIdent("nope", "t"),
		Columns: []*ast.ColumnDef{colDef("id", ast.TypeInt)},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownDatabase {
		t.Errorf("expected CodeUnknownDatabase, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_DecimalWithPrecision_Rejected(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	prec := 10
	stmt := &ast.CreateTableStmt{
		Table: ident("amounts"),
		Columns: []*ast.ColumnDef{
			{Name: "amount", Type: &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: &prec}},
		},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnsupportedDataType {
		t.Errorf("expected CodeUnsupportedDataType, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_DefaultTypeMismatch(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("bad_default"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, defaultConstraint(strLit("nope"), false)),
		},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidDefaultValue {
		t.Errorf("expected CodeInvalidDefaultValue, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_NegativeDefault(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("balances"),
		Columns: []*ast.ColumnDef{
			colDef("delta", ast.TypeInt, defaultConstraint(intLit("5"), true)),
		},
	}
	plan, err := pc.planCreateTable(stmt)
	if err != nil {
		t.Fatalf("planCreateTable: %v", err)
	}
	dv := plan.(*CreateTablePlan).Schema.Columns[0].DefaultValue
	if dv == nil || *dv != "-5" {
		t.Errorf("expected default \"-5\", got %+v", dv)
	}
}

func TestPlanDropTable_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, err := pc.planDropTable(&ast.DropTableStmt{Table: ident("orders")})
	if err != nil {
		t.Fatalf("planDropTable: %v", err)
	}
}

func TestPlanDropTable_Unknown(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, err := pc.planDropTable(&ast.DropTableStmt{Table: ident("nope")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownTable {
		t.Errorf("expected CodeUnknownTable, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanDropTable_IfExists_NoOp(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planDropTable(&ast.DropTableStmt{Table: ident("nope"), IfExists: true})
	if err != nil {
		t.Fatalf("planDropTable: %v", err)
	}
	if !plan.(*DropTablePlan).NoOp {
		t.Error("expected NoOp=true")
	}
}

func TestPlanAlterTable_AddColumn_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterAdd, Column: colDef("email", ast.TypeText)},
	}
	plan, err := pc.planAlterTable(stmt)
	if err != nil {
		t.Fatalf("planAlterTable: %v", err)
	}
	p := plan.(*AlterTablePlan)
	if len(p.NewSchema.Columns) != len(p.OldSchema.Columns)+1 {
		t.Errorf("expected one more column, old=%d new=%d", len(p.OldSchema.Columns), len(p.NewSchema.Columns))
	}
}

func TestPlanAlterTable_AddNotNullNoDefault_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterAdd, Column: colDef("required_field", ast.TypeInt, &ast.NotNullConstraint{})},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnsupportedAlter {
		t.Errorf("expected CodeUnsupportedAlter, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_AddDuplicateColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterAdd, Column: colDef("name", ast.TypeText)},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDuplicateColumn {
		t.Errorf("expected CodeDuplicateColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_ModifyColumn_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterModify, Column: varcharDef("name", 200)},
	}
	plan, err := pc.planAlterTable(stmt)
	if err != nil {
		t.Fatalf("planAlterTable: %v", err)
	}
	p := plan.(*AlterTablePlan)
	newCol := p.NewSchema.FindColumn("name")
	if newCol == nil || newCol.VarcharLen == nil || *newCol.VarcharLen != 200 {
		t.Errorf("expected name VARCHAR(200), got %+v", newCol)
	}
}

func TestPlanAlterTable_ModifyChangesType_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterModify, Column: colDef("name", ast.TypeInt)},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnsupportedAlter {
		t.Errorf("expected CodeUnsupportedAlter, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_ModifyUnknownColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterModify, Column: colDef("nope", ast.TypeInt)},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeColumnNotFound {
		t.Errorf("expected CodeColumnNotFound, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_DropColumn_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterDropColumn, DropName: "name"},
	}
	plan, err := pc.planAlterTable(stmt)
	if err != nil {
		t.Fatalf("planAlterTable: %v", err)
	}
	p := plan.(*AlterTablePlan)
	if len(p.NewSchema.ActiveColumns()) != len(p.OldSchema.ActiveColumns())-1 {
		t.Errorf("expected one fewer active column after drop")
	}
}

func TestPlanAlterTable_DropPKColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterDropColumn, DropName: "id"},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeCannotDropPKColumn {
		t.Errorf("expected CodeCannotDropPKColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_RenameColumn_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterRenameColumn, OldName: "name", NewName: "full_name"},
	}
	plan, err := pc.planAlterTable(stmt)
	if err != nil {
		t.Fatalf("planAlterTable: %v", err)
	}
	p := plan.(*AlterTablePlan)
	if p.NewSchema.FindColumn("full_name") == nil || p.NewSchema.FindColumn("name") != nil {
		t.Errorf("expected name renamed to full_name, got columns %+v", p.NewSchema.Columns)
	}
}

func TestPlanAlterTable_RenameColumn_NewNameTaken_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("users"),
		Action: &ast.AlterAction{Kind: ast.AlterRenameColumn, OldName: "name", NewName: "id"},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDuplicateColumn {
		t.Errorf("expected CodeDuplicateColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanAlterTable_RenameTable_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("orders"),
		Action: &ast.AlterAction{Kind: ast.AlterRenameTable, NewName: "purchase_orders"},
	}
	plan, err := pc.planAlterTable(stmt)
	if err != nil {
		t.Fatalf("planAlterTable: %v", err)
	}
	p := plan.(*RenameTablePlan)
	if p.OldName != "orders" || p.NewName != "purchase_orders" || p.Database != "shop" {
		t.Errorf("unexpected plan: %+v", p)
	}
}

func TestPlanAlterTable_RenameTable_TargetExists_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.AlterTableStmt{
		Table:  ident("orders"),
		Action: &ast.AlterAction{Kind: ast.AlterRenameTable, NewName: "users"},
	}
	_, err := pc.planAlterTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTableExists {
		t.Errorf("expected CodeTableExists, got err=%v diag=%+v", err, pc.diag)
	}
}

// sanity: confirm testCatalog's fixtures are what the tests above assume.
func TestFixtureSanityCheck(t *testing.T) {
	c := testCatalog()
	meta, err := c.GetTable("shop", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if len(meta.PrimaryKey) != 1 || meta.PrimaryKey[0] != "id" {
		t.Fatalf("expected shop.users PK [id], got %v", meta.PrimaryKey)
	}
	_ = catalog.NewEmptyCatalog() // keep the catalog import honest if fixtures change
}

func valuesRow(exprs ...ast.Expression) []*ast.SelectExpression {
	row := make([]*ast.SelectExpression, len(exprs))
	for i, e := range exprs {
		row[i] = &ast.SelectExpression{Expr: e}
	}
	return row
}

func TestPlanInsert_ExplicitColumns_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"id", "name"},
		Rows:    [][]*ast.SelectExpression{valuesRow(intLit("1"), strLit("Ada"))},
	}
	plan, err := pc.planInsert(stmt)
	if err != nil {
		t.Fatalf("planInsert: %v", err)
	}
	p := plan.(*InsertPlan)
	if len(p.Columns) != 2 || len(p.Rows) != 1 || len(p.Rows[0]) != 2 {
		t.Fatalf("unexpected plan: %+v", p)
	}
}

func TestPlanInsert_NoColumnList_DefaultsToAllActiveColumns(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table: ident("users"),
		Rows:  [][]*ast.SelectExpression{valuesRow(intLit("1"), strLit("Ada"))},
	}
	plan, err := pc.planInsert(stmt)
	if err != nil {
		t.Fatalf("planInsert: %v", err)
	}
	p := plan.(*InsertPlan)
	if len(p.Columns) != 2 || p.Columns[0].Name != "id" || p.Columns[1].Name != "name" {
		t.Errorf("expected default columns [id, name], got %+v", p.Columns)
	}
}

func TestPlanInsert_MultipleRows(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"id", "name"},
		Rows: [][]*ast.SelectExpression{
			valuesRow(intLit("1"), strLit("Ada")),
			valuesRow(intLit("2"), strLit("Grace")),
		},
	}
	plan, err := pc.planInsert(stmt)
	if err != nil {
		t.Fatalf("planInsert: %v", err)
	}
	if len(plan.(*InsertPlan).Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(plan.(*InsertPlan).Rows))
	}
}

func TestPlanInsert_UnknownColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"nope"},
		Rows:    [][]*ast.SelectExpression{valuesRow(intLit("1"))},
	}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownInsertColumn {
		t.Errorf("expected CodeUnknownInsertColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanInsert_DuplicateColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"id", "id"},
		Rows:    [][]*ast.SelectExpression{valuesRow(intLit("1"), intLit("2"))},
	}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDuplicateInsertColumn {
		t.Errorf("expected CodeDuplicateInsertColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanInsert_ColumnCountMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"id", "name"},
		Rows:    [][]*ast.SelectExpression{valuesRow(intLit("1"))},
	}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeColumnCountMismatch {
		t.Errorf("expected CodeColumnCountMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanInsert_TypeMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.InsertStmt{
		Table:   ident("users"),
		Columns: []string{"id", "name"},
		Rows:    [][]*ast.SelectExpression{valuesRow(strLit("not-an-id"), strLit("Ada"))},
	}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTypeMismatch {
		t.Errorf("expected CodeTypeMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanInsert_FromSelect_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	// orders is (id INT, user_id INT NOT NULL) -- both target columns are
	// INT, so a SELECT producing two INT columns lines up cleanly without
	// needing a third fixture table.
	source := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("o", "id"), ""), exprCol(qualifiedIdent("o", "user_id"), "")},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
	}
	stmt := &ast.InsertStmt{Table: ident("orders"), Columns: []string{"id", "user_id"}, Source: source}
	_, err := pc.planInsert(stmt)
	if err != nil {
		t.Fatalf("planInsert: %v", err)
	}
}

func TestPlanInsert_FromSelect_ColumnCountMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	source := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("o", "id"), "")},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
	}
	stmt := &ast.InsertStmt{Table: ident("users"), Columns: []string{"id", "name"}, Source: source}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeColumnCountMismatch {
		t.Errorf("expected CodeColumnCountMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanInsert_FromSelect_TypeMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	source := &ast.SelectStmt{
		Columns: []*ast.SelectColumn{exprCol(qualifiedIdent("o", "id"), ""), exprCol(qualifiedIdent("o", "id"), "")},
		From:    []*ast.TableRef{tableRef(primary(ident("orders"), "o"))},
	}
	// users is (id INT, name VARCHAR); source produces (INT, INT) -- second
	// column mismatches name's VARCHAR type.
	stmt := &ast.InsertStmt{Table: ident("users"), Columns: []string{"id", "name"}, Source: source}
	_, err := pc.planInsert(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTypeMismatch {
		t.Errorf("expected CodeTypeMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanUpdate_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.UpdateStmt{
		Table: ident("users"),
		Set:   []*ast.SetItem{{Column: ident("name"), Value: strLit("New Name")}},
		Where: &ast.WhereClause{Cond: &ast.ComparisonPredicate{Left: ident("id"), Op: utils.TOKEN_EQ, Right: intLit("1")}},
	}
	plan, err := pc.planUpdate(stmt)
	if err != nil {
		t.Fatalf("planUpdate: %v", err)
	}
	p := plan.(*UpdatePlan)
	if len(p.Assignments) != 1 || p.Where == nil {
		t.Errorf("unexpected plan: %+v", p)
	}
}

func TestPlanUpdate_NoWhere_UpdatesEveryRow(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.UpdateStmt{
		Table: ident("users"),
		Set:   []*ast.SetItem{{Column: ident("name"), Value: strLit("Everyone")}},
	}
	plan, err := pc.planUpdate(stmt)
	if err != nil {
		t.Fatalf("planUpdate: %v", err)
	}
	if plan.(*UpdatePlan).Where != nil {
		t.Error("expected nil Where with no WHERE clause")
	}
}

func TestPlanUpdate_UnknownColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.UpdateStmt{
		Table: ident("users"),
		Set:   []*ast.SetItem{{Column: ident("nope"), Value: strLit("x")}},
	}
	_, err := pc.planUpdate(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownColumn {
		t.Errorf("expected CodeUnknownColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanUpdate_TypeMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.UpdateStmt{
		Table: ident("users"),
		Set:   []*ast.SetItem{{Column: ident("id"), Value: strLit("not-a-number")}},
	}
	_, err := pc.planUpdate(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTypeMismatch {
		t.Errorf("expected CodeTypeMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanDelete_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.DeleteStmt{
		Table: ident("users"),
		Where: &ast.WhereClause{Cond: &ast.ComparisonPredicate{Left: ident("id"), Op: utils.TOKEN_EQ, Right: intLit("1")}},
	}
	plan, err := pc.planDelete(stmt)
	if err != nil {
		t.Fatalf("planDelete: %v", err)
	}
	if plan.(*DeletePlan).Where == nil {
		t.Error("expected a non-nil Where")
	}
}

func TestPlanDelete_NoWhere_DeletesEveryRow(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	plan, err := pc.planDelete(&ast.DeleteStmt{Table: ident("users")})
	if err != nil {
		t.Fatalf("planDelete: %v", err)
	}
	if plan.(*DeletePlan).Where != nil {
		t.Error("expected nil Where with no WHERE clause")
	}
}

func TestPlanDelete_UnknownTable_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, err := pc.planDelete(&ast.DeleteStmt{Table: ident("nope")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownTable {
		t.Errorf("expected CodeUnknownTable, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanUpdate_DuplicateSetColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.UpdateStmt{
		Table: ident("users"),
		Set: []*ast.SetItem{
			{Column: ident("name"), Value: strLit("A")},
			{Column: ident("name"), Value: strLit("B")},
		},
	}
	_, err := pc.planUpdate(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeDuplicateSetColumn {
		t.Errorf("expected CodeDuplicateSetColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_ForeignKeyUnknownTable_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("new_table"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, &ast.PrimaryKeyConstraint{}),
			colDef("ref_id", ast.TypeInt, &ast.ReferencesConstraint{Table: "nonexistent", Column: "id"}),
		},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidForeignKey {
		t.Errorf("expected CodeInvalidForeignKey, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_ForeignKeyUnknownColumn_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("new_table"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, &ast.PrimaryKeyConstraint{}),
			colDef("ref_id", ast.TypeInt, &ast.ReferencesConstraint{Table: "users", Column: "nonexistent"}),
		},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidForeignKey {
		t.Errorf("expected CodeInvalidForeignKey, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestPlanCreateTable_ForeignKeyValid_PopulatesReferencedDB(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("new_table"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, &ast.PrimaryKeyConstraint{}),
			colDef("user_id", ast.TypeInt, &ast.ReferencesConstraint{Table: "users", Column: "id"}),
		},
	}
	plan, err := pc.planCreateTable(stmt)
	if err != nil {
		t.Fatalf("planCreateTable: %v", err)
	}
	schema := plan.(*CreateTablePlan).Schema
	fk := schema.Columns[1].ForeignKey
	if fk == nil {
		t.Fatal("expected ForeignKey to be set")
	}
	if fk.ReferencedDB != "shop" || fk.ReferencedTable != "users" || fk.ReferencedColumn != "id" {
		t.Errorf("unexpected FK: %+v", fk)
	}
}

func TestPlanCreateTable_ForeignKeyTypeMismatch_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	stmt := &ast.CreateTableStmt{
		Table: ident("new_table"),
		Columns: []*ast.ColumnDef{
			colDef("id", ast.TypeInt, &ast.PrimaryKeyConstraint{}),
			// INT referencing VARCHAR(100) users.name — type-incompatible.
			colDef("user_name", ast.TypeInt, &ast.ReferencesConstraint{Table: "users", Column: "name"}),
		},
	}
	_, err := pc.planCreateTable(stmt)
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidForeignKey {
		t.Errorf("expected CodeInvalidForeignKey for type-incompatible FK, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- InsertPlan.Validate -------------------------------------------------

func TestInsertPlan_Validate_BothNil_Errors(t *testing.T) {
	plan := &InsertPlan{}
	if err := plan.Validate(); err == nil {
		t.Error("expected Validate to fail when both Rows and Source are nil")
	}
}

func TestInsertPlan_Validate_RowsOnly_Succeeds(t *testing.T) {
	plan := &InsertPlan{Rows: [][]ResolvedExpr{{}}}
	if err := plan.Validate(); err != nil {
		t.Errorf("expected Validate to succeed with Rows only, got %v", err)
	}
}

func TestInsertPlan_Validate_SourceOnly_Succeeds(t *testing.T) {
	plan := &InsertPlan{Source: &QueryPlan{}}
	if err := plan.Validate(); err != nil {
		t.Errorf("expected Validate to succeed with Source only, got %v", err)
	}
}

func TestInsertPlan_Validate_BothSet_Errors(t *testing.T) {
	plan := &InsertPlan{Rows: [][]ResolvedExpr{{}}, Source: &QueryPlan{}}
	if err := plan.Validate(); err == nil {
		t.Error("expected Validate to fail when both Rows and Source are set")
	}
}

// --- context cancellation ------------------------------------------------

func TestPlan_CancelledContext_ReturnsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop", Ctx: ctx}, nil)
	_, err := pc.planStatement(&ast.SelectStmt{
		Columns: []*ast.SelectColumn{{Star: true}},
		From:    []*ast.TableRef{tableRef(primary(ident("users"), "u"))},
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
