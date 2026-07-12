package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
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
