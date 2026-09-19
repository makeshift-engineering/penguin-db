package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// testCatalog builds a catalog seeded with two databases and a handful of
// tables covering the shapes resolution needs to handle: an unqualified
// active-database table, a table only reachable via explicit qualifier, and
// two tables sharing a column name (for ambiguity testing).
func testCatalog() *catalog.Catalog {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(&catalog.DatabaseMeta{Name: "shop"})
	c.ApplyCreateDatabase(&catalog.DatabaseMeta{Name: "archive"})

	c.ApplyCreateTable(&catalog.TableMeta{
		Database: "shop",
		Name:     "users",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "name", Type: ast.TypeVarchar, VarcharLen: intPtr(100)},
		},
		PrimaryKey: []string{"id"},
		Version:    1,
	})

	c.ApplyCreateTable(&catalog.TableMeta{
		Database: "shop",
		Name:     "orders",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, PrimaryKey: true},
			{Name: "user_id", Type: ast.TypeInt, NotNull: true},
		},
		PrimaryKey: []string{"id"},
		Version:    1,
	})

	c.ApplyCreateTable(&catalog.TableMeta{
		Database: "archive",
		Name:     "logs",
		Columns:  []catalog.ColumnMeta{{Name: "id", Type: ast.TypeBigInt}},
		Version:  1,
	})

	return c
}

func intPtr(v int) *int { return &v }

func ident(name string) *ast.Identifier {
	return &ast.Identifier{Name: name}
}

func qualifiedIdent(qualifier, name string) *ast.Identifier {
	return &ast.Identifier{Qualifier: qualifier, Name: name}
}

func tableRef(primary *ast.TablePrimary, joins ...*ast.JoinClause) *ast.TableRef {
	return &ast.TableRef{Primary: primary, Joins: joins}
}

func primary(name *ast.Identifier, alias string) *ast.TablePrimary {
	return &ast.TablePrimary{Name: name, Alias: alias}
}

func TestResolveDatabaseName_ExplicitQualifier(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	db, err := pc.resolveDatabaseName("archive", ident("logs"))
	if err != nil {
		t.Fatalf("resolveDatabaseName: %v", err)
	}
	if db != "archive" {
		t.Errorf("expected explicit qualifier to win, got %q", db)
	}
}

func TestResolveDatabaseName_SessionFallback(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	db, err := pc.resolveDatabaseName("", ident("users"))
	if err != nil {
		t.Fatalf("resolveDatabaseName: %v", err)
	}
	if db != "shop" {
		t.Errorf("expected session's active database, got %q", db)
	}
}

func TestResolveDatabaseName_NoActiveDatabase(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveDatabaseName("", ident("users"))
	if err == nil {
		t.Fatal("expected an error when no database is qualified or active")
	}
	if len(pc.diag) != 1 || pc.diag[0].Code != CodeNoActiveDatabase {
		t.Errorf("expected one CodeNoActiveDatabase diagnostic, got %+v", pc.diag)
	}
}

func TestResolveTableIdentifier_Success(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	meta, db, err := pc.resolveTableIdentifier(ident("users"))
	if err != nil {
		t.Fatalf("resolveTableIdentifier: %v", err)
	}
	if db != "shop" || meta.Name != "users" {
		t.Errorf("expected shop.users, got %s.%s", db, meta.Name)
	}
}

func TestResolveTableIdentifier_UnknownDatabase(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, _, err := pc.resolveTableIdentifier(qualifiedIdent("nope", "users"))
	if err == nil {
		t.Fatal("expected an error for an unknown database")
	}
	if len(pc.diag) != 1 || pc.diag[0].Code != CodeUnknownDatabase {
		t.Errorf("expected one CodeUnknownDatabase diagnostic, got %+v", pc.diag)
	}
}

func TestResolveTableIdentifier_UnknownTable(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, _, err := pc.resolveTableIdentifier(ident("nope"))
	if err == nil {
		t.Fatal("expected an error for an unknown table")
	}
	if len(pc.diag) != 1 || pc.diag[0].Code != CodeUnknownTable {
		t.Errorf("expected one CodeUnknownTable diagnostic, got %+v", pc.diag)
	}
}

func TestBuildScope_SingleTableNoAlias(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(primary(ident("users"), "")),
	})
	if pc.diag.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", pc.diag)
	}
	if len(scope.Tables()) != 1 {
		t.Fatalf("expected 1 table in scope, got %d", len(scope.Tables()))
	}
	if _, ok := scope.byBinding["users"]; !ok {
		t.Error("expected table bound under its own name")
	}
}

func TestBuildScope_Alias(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(primary(ident("users"), "u")),
	})
	if pc.diag.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", pc.diag)
	}
	if _, ok := scope.byBinding["u"]; !ok {
		t.Error("expected table bound under its alias")
	}
	if _, ok := scope.byBinding["users"]; ok {
		t.Error("aliased table should not also be bound under its bare name")
	}
}

func TestBuildScope_JoinChain(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(
			primary(ident("users"), "u"),
			&ast.JoinClause{
				Type:  ast.JoinInner,
				Right: primary(ident("orders"), "o"),
				On: &ast.ComparisonPredicate{
					Left:  qualifiedIdent("u", "id"),
					Op:    utils.TOKEN_EQ,
					Right: qualifiedIdent("o", "user_id"),
				},
			},
		),
	})
	if pc.diag.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", pc.diag)
	}
	if len(scope.Tables()) != 2 {
		t.Fatalf("expected 2 tables in scope, got %d", len(scope.Tables()))
	}
}

func TestBuildScope_DuplicateBinding(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(primary(ident("users"), "")),
		tableRef(primary(ident("users"), "")),
	})
	if len(scope.Tables()) != 1 {
		t.Errorf("expected only the first binding to register, got %d tables", len(scope.Tables()))
	}
	if !pc.diag.HasErrors() {
		t.Fatal("expected a duplicate-binding diagnostic")
	}
	if pc.diag[0].Code != CodeDuplicateTableBinding {
		t.Errorf("expected CodeDuplicateTableBinding, got %v", pc.diag[0].Code)
	}
}

func TestBuildScope_UnsupportedJoinType(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(
			primary(ident("users"), "u"),
			&ast.JoinClause{
				Type:  ast.JoinLeft,
				Right: primary(ident("orders"), "o"),
				On: &ast.ComparisonPredicate{
					Left:  qualifiedIdent("u", "id"),
					Op:    utils.TOKEN_EQ,
					Right: qualifiedIdent("o", "user_id"),
				},
			},
		),
	})
	if len(scope.Tables()) != 1 {
		t.Errorf("expected only the base table to register, got %d tables", len(scope.Tables()))
	}
	if !pc.diag.HasErrors() || pc.diag[0].Code != CodeUnsupportedJoinType {
		t.Errorf("expected CodeUnsupportedJoinType, got %+v", pc.diag)
	}
}

func TestBuildScope_ParenNesting(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		{Paren: tableRef(primary(ident("users"), "u"))},
	})
	if pc.diag.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", pc.diag)
	}
	if len(scope.Tables()) != 1 {
		t.Fatalf("expected 1 table in scope, got %d", len(scope.Tables()))
	}
}

// --- resolveColumn -------------------------------------------------------

func TestResolveColumn_BareUnique(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{tableRef(primary(ident("users"), "u"))})

	col, err := pc.resolveColumn(scope, ident("name"))
	if err != nil {
		t.Fatalf("resolveColumn: %v", err)
	}
	if col.Name != "name" || col.Table != "users" || col.Index != 1 {
		t.Errorf("unexpected resolution: %+v", col)
	}
}

func TestResolveColumn_Qualified(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{tableRef(primary(ident("users"), "u"))})

	col, err := pc.resolveColumn(scope, qualifiedIdent("u", "id"))
	if err != nil {
		t.Fatalf("resolveColumn: %v", err)
	}
	if col.Name != "id" || col.Index != 0 {
		t.Errorf("unexpected resolution: %+v", col)
	}
}

func TestResolveColumn_Ambiguous(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{
		tableRef(
			primary(ident("users"), "u"),
			&ast.JoinClause{
				Type:  ast.JoinInner,
				Right: primary(ident("orders"), "o"),
				On: &ast.ComparisonPredicate{
					Left:  qualifiedIdent("u", "id"),
					Op:    utils.TOKEN_EQ,
					Right: qualifiedIdent("o", "id"),
				},
			},
		),
	})

	_, err := pc.resolveColumn(scope, ident("id"))
	if err == nil {
		t.Fatal("expected an ambiguous-column error")
	}
	if pc.diag[len(pc.diag)-1].Code != CodeAmbiguousColumn {
		t.Errorf("expected CodeAmbiguousColumn, got %v", pc.diag[len(pc.diag)-1].Code)
	}
}

func TestResolveColumn_Unknown(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{tableRef(primary(ident("users"), "u"))})

	_, err := pc.resolveColumn(scope, ident("nope"))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownColumn {
		t.Errorf("expected CodeUnknownColumn, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveColumn_UnknownBinding(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := pc.buildScope([]*ast.TableRef{tableRef(primary(ident("users"), "u"))})

	_, err := pc.resolveColumn(scope, qualifiedIdent("z", "id"))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownTableBinding {
		t.Errorf("expected CodeUnknownTableBinding, got err=%v diag=%+v", err, pc.diag)
	}
}
