package planner

import (
	"errors"
	"slices"
	"strings"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// tableExists reports whether db.table exists in the catalog, without
// emitting any diagnostic. It's used for IF EXISTS / IF NOT EXISTS
// handling, where a missing table is an expected outcome, not an error —
// and it deliberately treats "database doesn't exist" and "table doesn't
// exist within it" the same way, since both mean "there's nothing here,"
// which is exactly what those clauses care about.
func (pc *planContext) tableExists(db, table string) bool {
	_, err := pc.catalog.GetTable(db, table)
	return err == nil
}

func (pc *planContext) planCreateDatabase(stmt *ast.CreateDatabaseStmt) (Plan, error) {
	exists := pc.catalog.DatabaseExists(stmt.Name)
	if exists && stmt.IfNotExists {
		return &CreateDatabasePlan{Name: stmt.Name, NoOp: true}, nil
	}
	if exists {
		return nil, pc.errorf(stmt.Span(), CodeDatabaseExists, "database %q already exists", stmt.Name)
	}
	return &CreateDatabasePlan{Name: stmt.Name}, nil
}

func (pc *planContext) planUseDatabase(stmt *ast.UseDatabaseStmt) (Plan, error) {
	if !pc.catalog.DatabaseExists(stmt.Name) {
		return nil, pc.errorf(stmt.Span(), CodeUnknownDatabase, "database %q does not exist", stmt.Name)
	}
	return &UseDatabasePlan{Name: stmt.Name}, nil
}

func (pc *planContext) planDropDatabase(stmt *ast.DropDatabaseStmt) (Plan, error) {
	tables, err := pc.catalog.ListTables(stmt.Name)
	if err != nil {
		if stmt.IfExists && errors.Is(err, catalog.ErrDatabaseNotFound) {
			return &DropDatabasePlan{Name: stmt.Name, NoOp: true}, nil
		}
		return nil, pc.errorf(stmt.Span(), CodeUnknownDatabase, "database %q does not exist", stmt.Name)
	}
	return &DropDatabasePlan{Name: stmt.Name, Tables: tables}, nil
}

func (pc *planContext) planCreateTable(stmt *ast.CreateTableStmt) (Plan, error) {
	db, err := pc.resolveDatabaseName(stmt.Table.Qualifier, stmt.Table)
	if err != nil {
		return nil, err
	}
	if !pc.catalog.DatabaseExists(db) {
		return nil, pc.errorf(stmt.Table.Span(), CodeUnknownDatabase, "database %q does not exist", db)
	}

	if pc.tableExists(db, stmt.Table.Name) {
		if stmt.IfNotExists {
			return &CreateTablePlan{Database: db, Table: stmt.Table.Name, NoOp: true}, nil
		}
		return nil, pc.errorf(stmt.Table.Span(), CodeTableExists, "table %q already exists in database %q", stmt.Table.Name, db)
	}

	schema, err := pc.buildTableMeta(db, stmt.Table.Name, stmt.Columns)
	if err != nil {
		return nil, err
	}
	return &CreateTablePlan{Database: db, Table: stmt.Table.Name, Schema: schema}, nil
}

func (pc *planContext) planDropTable(stmt *ast.DropTableStmt) (Plan, error) {
	db, err := pc.resolveDatabaseName(stmt.Table.Qualifier, stmt.Table)
	if err != nil {
		return nil, err
	}

	if !pc.tableExists(db, stmt.Table.Name) {
		if stmt.IfExists {
			return &DropTablePlan{Database: db, Table: stmt.Table.Name, NoOp: true}, nil
		}
		return nil, pc.errorf(stmt.Table.Span(), CodeUnknownTable, "table %q does not exist in database %q", stmt.Table.Name, db)
	}
	return &DropTablePlan{Database: db, Table: stmt.Table.Name}, nil
}

func (pc *planContext) planAlterTable(stmt *ast.AlterTableStmt) (Plan, error) {
	oldMeta, db, err := pc.resolveTableIdentifier(stmt.Table)
	if err != nil {
		return nil, err
	}

	if stmt.Action.Kind == ast.AlterRenameTable {
		return pc.planRenameTable(db, oldMeta, stmt.Action)
	}
	return pc.planAlterSchema(oldMeta, stmt.Action)
}

func (pc *planContext) planRenameTable(db string, oldMeta *catalog.TableMeta, action *ast.AlterAction) (Plan, error) {
	if pc.tableExists(db, action.NewName) {
		return nil, pc.errorf(action.Span(), CodeTableExists, "table %q already exists in database %q", action.NewName, db)
	}
	return &RenameTablePlan{Database: db, OldName: oldMeta.Name, NewName: action.NewName, Schema: oldMeta}, nil
}

// planAlterSchema handles every ALTER TABLE action except a table rename by
// cloning the table's current schema and applying the action to the clone,
// producing the old/new schema pair catalog.BuildAlterTableOps diffs.
func (pc *planContext) planAlterSchema(oldMeta *catalog.TableMeta, action *ast.AlterAction) (Plan, error) {
	newMeta := oldMeta.Clone()

	switch action.Kind {
	case ast.AlterAdd:
		col, err := pc.buildColumnMeta(oldMeta.Database, action.Column)
		if err != nil {
			return nil, err
		}
		if oldMeta.FindColumn(col.Name) != nil {
			return nil, pc.errorf(action.Column.Span(), CodeDuplicateColumn, "column %q already exists", col.Name)
		}
		if col.Unique {
			return nil, pc.errorf(action.Column.Span(), CodeUnsupportedAlter, "adding a UNIQUE column is not supported in v1")
		}
		if col.PrimaryKey {
			return nil, pc.errorf(action.Column.Span(), CodeUnsupportedAlter, "adding a PRIMARY KEY column is not supported in v1")
		}
		if col.NotNull && col.DefaultValue == nil {
			return nil, pc.errorf(action.Column.Span(), CodeUnsupportedAlter, "a new NOT NULL column requires a DEFAULT value")
		}
		newMeta.Columns = append(newMeta.Columns, *col)

	case ast.AlterModify:
		existing := oldMeta.FindColumn(action.Column.Name)
		if existing == nil {
			return nil, pc.errorf(action.Column.Span(), CodeColumnNotFound, "column %q does not exist", action.Column.Name)
		}
		col, err := pc.buildColumnMeta(oldMeta.Database, action.Column)
		if err != nil {
			return nil, err
		}
		if col.Type != existing.Type {
			return nil, pc.errorf(action.Column.Span(), CodeUnsupportedAlter, "changing a column's data type is not supported in v1")
		}
		if col.PrimaryKey != existing.PrimaryKey {
			return nil, pc.errorf(action.Column.Span(), CodeUnsupportedAlter, "changing a column's PRIMARY KEY status via MODIFY is not supported in v1")
		}
		replaceActiveColumn(newMeta, col.Name, *col)

	case ast.AlterDropColumn:
		if oldMeta.FindColumn(action.DropName) == nil {
			return nil, pc.errorf(action.Span(), CodeColumnNotFound, "column %q does not exist", action.DropName)
		}
		if slices.Contains(oldMeta.PrimaryKey, action.DropName) {
			return nil, pc.errorf(action.Span(), CodeCannotDropPKColumn, "cannot drop primary key column %q", action.DropName)
		}
		for i := range newMeta.Columns {
			if newMeta.Columns[i].Name == action.DropName && !newMeta.Columns[i].Dropped {
				newMeta.Columns[i].Dropped = true
				break
			}
		}

	case ast.AlterRenameColumn:

		if oldMeta.FindColumn(action.OldName) == nil {
			return nil, pc.errorf(action.Span(), CodeColumnNotFound, "column %q does not exist", action.OldName)
		}
		if oldMeta.FindColumn(action.NewName) != nil {
			return nil, pc.errorf(action.Span(), CodeDuplicateColumn, "column %q already exists", action.NewName)
		}
		for i := range newMeta.Columns {
			if newMeta.Columns[i].Name == action.OldName && !newMeta.Columns[i].Dropped {
				newMeta.Columns[i].Name = action.NewName
				break
			}
		}
		for i, pkName := range newMeta.PrimaryKey {
			if pkName == action.OldName {
				newMeta.PrimaryKey[i] = action.NewName
			}
		}

	default:
		return nil, pc.errorf(action.Span(), CodeInvalidAlterAction, "unsupported ALTER TABLE action")
	}

	return &AlterTablePlan{OldSchema: oldMeta, NewSchema: newMeta}, nil
}

// replaceActiveColumn overwrites the named active column's metadata in
// place, preserving its position so column indices elsewhere stay valid.
func replaceActiveColumn(meta *catalog.TableMeta, name string, col catalog.ColumnMeta) {
	for i := range meta.Columns {
		if meta.Columns[i].Name == name && !meta.Columns[i].Dropped {
			meta.Columns[i] = col
			return
		}
	}
}

// buildTableMeta constructs a catalog-ready TableMeta from a CREATE TABLE
// statement's column definitions. It does not set Version or CreatedAt —
// those are execution-time facts the executor fills in when it actually
// persists the table via catalog.BuildCreateTableOps.
func (pc *planContext) buildTableMeta(db, table string, defs []*ast.ColumnDef) (*catalog.TableMeta, error) {
	meta := &catalog.TableMeta{Database: db, Name: table}
	seen := make(map[string]bool, len(defs))
	var firstErr error

	for _, def := range defs {
		if err := pc.checkContext(); err != nil {
			return nil, err
		}
		if seen[def.Name] {
			recordErr(&firstErr, pc.errorf(def.Span(), CodeDuplicateColumn, "duplicate column name %q", def.Name))
			continue
		}
		seen[def.Name] = true

		col, err := pc.buildColumnMeta(db, def)
		if err != nil {
			recordErr(&firstErr, err)
			continue
		}
		meta.Columns = append(meta.Columns, *col)
		if col.PrimaryKey {
			meta.PrimaryKey = append(meta.PrimaryKey, col.Name)
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	if len(meta.PrimaryKey) == 0 {
		meta.HasSnowflakeID = true
	}
	return meta, nil
}

// buildColumnMeta resolves a single column definition's type and walks its
// constraint list into a catalog-ready ColumnMeta. db is the database the
// owning table lives in — used to validate REFERENCES targets.
func (pc *planContext) buildColumnMeta(db string, def *ast.ColumnDef) (*catalog.ColumnMeta, error) {
	dtype, err := pc.resolveDataType(def.Type)
	if err != nil {
		return nil, err
	}

	col := &catalog.ColumnMeta{Name: def.Name, Type: dtype.Type, VarcharLen: dtype.VarcharLen}

	var firstErr error
	for _, constr := range def.Constraints {
		switch c := constr.(type) {
		case *ast.PrimaryKeyConstraint:
			col.PrimaryKey = true
			col.NotNull = true // PRIMARY KEY implies NOT NULL
		case *ast.UniqueConstraint:
			col.Unique = true
		case *ast.NotNullConstraint:
			col.NotNull = true
		case *ast.NullConstraint:
			// Explicit NULL: no-op, NotNull already defaults to false.
		case *ast.DefaultConstraint:
			value, err := pc.resolveDefaultValue(c.Value, col.Type)
			if err != nil {
				recordErr(&firstErr, err)
				continue
			}
			col.DefaultValue = value
		case *ast.ForeignRef, *ast.ReferencesConstraint:
			var targetTable, targetCol string
			if ref, ok := constr.(*ast.ForeignRef); ok {
				targetTable, targetCol = ref.Table, ref.Column
			} else if ref, ok := constr.(*ast.ReferencesConstraint); ok {
				targetTable, targetCol = ref.Table, ref.Column
			}
			fk, fkErr := pc.validateForeignKey(db, targetTable, targetCol, col.Type, constr)
			if fkErr != nil {
				recordErr(&firstErr, fkErr)
				continue
			}
			col.ForeignKey = fk
			// A foreign key can only target a table in the same database as
			// the referencing column.
		default:
			recordErr(&firstErr, pc.errorf(constr.Span(), CodeUnsupportedConstraint, "unsupported column constraint %T", constr))
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return col, nil
}

type resolvedDataType struct {
	Type       ast.DataTypeKind
	VarcharLen *int
}

// resolveDataType validates a parsed data type against what the catalog
// schema can actually represent. catalog.ColumnMeta has no fields for
// DECIMAL precision or scale, so a DECIMAL with either specified is
// rejected here rather than silently discarding information the person
// explicitly asked for.
func (pc *planContext) resolveDataType(dt *ast.DataType) (*resolvedDataType, error) {
	if dt.Kind == ast.TypeDecimal && (dt.DecimalPrec != nil || dt.DecimalScale != nil) {
		return nil, pc.errorf(
			dt.Span(), CodeUnsupportedDataType,
			"DECIMAL precision and scale are not yet supported by the catalog; use DECIMAL without arguments",
		)
	}
	return &resolvedDataType{Type: dt.Kind, VarcharLen: dt.VarcharLen}, nil
}

// resolveDefaultValue type-checks a DEFAULT literal against its column's
// declared type (reusing typesCompatible from typecheck.go) and serializes
// it to the string form catalog.ColumnMeta.DefaultValue stores.
//
// DEFAULT NULL resolves to (nil, nil): DefaultValue has no way to
// distinguish "explicit NULL default" from "no default at all", so the two
// necessarily collapse into the same representation.
func (pc *planContext) resolveDefaultValue(lit *ast.SignedLiteral, colType ast.DataTypeKind) (*string, error) {
	if _, ok := lit.Value.(*ast.NullLiteral); ok {
		return nil, nil
	}

	if lit.Negative {
		switch lit.Value.(type) {
		case *ast.IntegerLiteral, *ast.FloatLiteral:
		default:
			return nil, pc.errorf(lit.Span(), CodeInvalidDefaultValue, "a sign is only valid on a numeric DEFAULT value")
		}
	}

	var text string
	var litType ast.DataTypeKind
	switch v := lit.Value.(type) {
	case *ast.IntegerLiteral:
		text, litType = v.Value, ast.TypeBigInt
	case *ast.FloatLiteral:
		text, litType = v.Value, ast.TypeDouble
	case *ast.StringLiteral:
		text, litType = v.Value, ast.TypeText
	case *ast.BooleanLiteral:
		text, litType = strings.ToUpper(v.Value), ast.TypeBoolean
	default:
		return nil, pc.errorf(lit.Span(), CodeInvalidDefaultValue, "unsupported DEFAULT literal type %T", v)
	}

	if !typesCompatible(litType, colType) {
		return nil, pc.errorf(
			lit.Span(), CodeInvalidDefaultValue,
			"DEFAULT value type %s is not compatible with column type %s", typeName(litType), typeName(colType),
		)
	}

	if lit.Negative {
		text = "-" + text
	}
	return &text, nil
}

func (pc *planContext) planInsert(stmt *ast.InsertStmt) (Plan, error) {
	meta, db, err := pc.resolveTableIdentifier(stmt.Table)
	if err != nil {
		return nil, err
	}

	targetCols, err := pc.resolveInsertColumns(meta, stmt.Columns, stmt.Table)
	if err != nil {
		return nil, err
	}

	plan := &InsertPlan{Database: db, Table: meta.Name, Schema: meta, Columns: targetCols}

	if stmt.Source != nil {
		source, err := pc.planSelect(stmt.Source)
		if err != nil {
			return nil, err
		}
		if len(source.Columns) != len(targetCols) {
			return nil, pc.errorf(
				stmt.Source.Span(), CodeColumnCountMismatch,
				"INSERT has %d target columns but SELECT produces %d", len(targetCols), len(source.Columns),
			)
		}
		for i, col := range targetCols {
			if !typesCompatible(source.Columns[i].Type, col.Type) {
				return nil, pc.errorf(
					stmt.Source.Span(), CodeTypeMismatch,
					"column %d: SELECT produces %s, target column %q is %s",
					i+1, typeName(source.Columns[i].Type), col.Name, typeName(col.Type),
				)
			}
		}
		plan.Source = source
		if err := plan.Validate(); err != nil {
			return nil, err
		}
		return plan, nil
	}

	rows := make([][]ResolvedExpr, 0, len(stmt.Rows))
	var firstErr error
	for _, row := range stmt.Rows {
		if err := pc.checkContext(); err != nil {
			return nil, err
		}
		if len(row) != len(targetCols) {
			recordErr(&firstErr, pc.errorf(
				stmt.Table.Span(), CodeColumnCountMismatch,
				"INSERT has %d target columns but a VALUES row has %d", len(targetCols), len(row),
			))
			continue
		}

		resolvedRow := make([]ResolvedExpr, len(row))
		rowOK := true
		for i, val := range row {
			expr, err := pc.resolveSelectExpression(newScope(), val)
			if err != nil {
				rowOK = false
				recordErr(&firstErr, err)
				continue
			}
			if !exprsCompatible(expr, &ResolvedColumnRef{Column: targetCols[i]}) {
				recordErr(&firstErr, pc.errorf(
					val.Span(), CodeTypeMismatch,
					"value type %s is not compatible with column %q (%s)",
					exprTypeName(expr), targetCols[i].Name, typeName(targetCols[i].Type),
				))
				rowOK = false
				continue
			}
			resolvedRow[i] = expr
		}
		if rowOK {
			rows = append(rows, resolvedRow)
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}
	plan.Rows = rows
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

// resolveInsertColumns determines the ordered target column list for an
// INSERT. An explicit column list is validated against the table's active
// columns (existence, no duplicates); an absent one defaults to every
// active column in declaration order, matching how INSERT INTO t VALUES
// (...) with no column list is interpreted. names carries no positional
// span of its own (ast.InsertStmt.Columns is a plain []string), so
// diagnostics here point at the statement's table identifier — the closest
// span the AST actually provides.
func (pc *planContext) resolveInsertColumns(meta *catalog.TableMeta, names []string, tableID *ast.Identifier) ([]ResolvedColumn, error) {
	active := meta.ActiveColumns()

	if len(names) == 0 {
		cols := make([]ResolvedColumn, len(active))
		for i, c := range active {
			cols[i] = *resolvedColumnFrom(meta, c, i)
		}
		return cols, nil
	}

	indexByName := make(map[string]int, len(active))
	for i, c := range active {
		indexByName[c.Name] = i
	}

	cols := make([]ResolvedColumn, 0, len(names))
	seen := make(map[string]bool, len(names))
	var firstErr error
	for _, name := range names {
		if seen[name] {
			recordErr(&firstErr, pc.errorf(tableID.Span(), CodeDuplicateInsertColumn, "duplicate column %q in INSERT column list", name))
			continue
		}
		seen[name] = true

		idx, ok := indexByName[name]
		if !ok {
			recordErr(&firstErr, pc.errorf(tableID.Span(), CodeUnknownInsertColumn, "column %q does not exist on table %q", name, meta.Name))
			continue
		}
		cols = append(cols, *resolvedColumnFrom(meta, active[idx], idx))
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return cols, nil
}

func (pc *planContext) planUpdate(stmt *ast.UpdateStmt) (Plan, error) {
	meta, db, err := pc.resolveTableIdentifier(stmt.Table)
	if err != nil {
		return nil, err
	}

	scope := newSingleTableScope(meta, stmt.Table.Name)

	assignments := make([]Assignment, 0, len(stmt.Set))
	seenCols := make(map[string]bool, len(stmt.Set))
	var firstErr error
	for _, item := range stmt.Set {
		col, err := pc.resolveColumn(scope, item.Column)
		if err != nil {
			recordErr(&firstErr, err)
			continue
		}
		if seenCols[col.Name] {
			recordErr(&firstErr, pc.errorf(item.Column.Span(), CodeDuplicateSetColumn, "column %q is assigned more than once in SET clause", col.Name))
			continue
		}
		seenCols[col.Name] = true
		value, err := pc.resolveExpr(scope, item.Value)
		if err != nil {
			recordErr(&firstErr, err)
			continue
		}
		if !exprsCompatible(value, &ResolvedColumnRef{Column: *col}) {
			recordErr(&firstErr, pc.errorf(
				item.Value.Span(), CodeTypeMismatch,
				"value type %s is not compatible with column %q (%s)", exprTypeName(value), col.Name, typeName(col.Type),
			))
			continue
		}
		assignments = append(assignments, Assignment{Column: *col, Value: value})
	}

	var where ResolvedCond
	if stmt.Where != nil {
		where, err = pc.resolveCond(scope, stmt.Where.Cond)
		recordErr(&firstErr, err)
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return &UpdatePlan{Database: db, Table: meta.Name, Schema: meta, Assignments: assignments, Where: where}, nil
}

func (pc *planContext) planDelete(stmt *ast.DeleteStmt) (Plan, error) {
	meta, db, err := pc.resolveTableIdentifier(stmt.Table)
	if err != nil {
		return nil, err
	}

	var where ResolvedCond
	if stmt.Where != nil {
		scope := newSingleTableScope(meta, stmt.Table.Name)
		where, err = pc.resolveCond(scope, stmt.Where.Cond)
		if err != nil {
			return nil, err
		}
	}

	return &DeletePlan{Database: db, Table: meta.Name, Schema: meta, Where: where}, nil
}

// validateForeignKey checks that a REFERENCES target table and column exist
// in the catalog within the given database, and that the referenced column's
// type is compatible with the referencing column's type. Foreign keys can
// only reference tables in the same database as the referencing column.
func (pc *planContext) validateForeignKey(db, refTable, refColumn string, srcType ast.DataTypeKind, node ast.Clause) (*catalog.ForeignKeyRef, error) {
	meta, err := pc.catalog.GetTable(db, refTable)
	if err != nil {
		return nil, pc.errorf(
			node.Span(), CodeInvalidForeignKey,
			"foreign key references unknown table %q in database %q", refTable, db,
		)
	}
	targetCol := meta.FindColumn(refColumn)
	if targetCol == nil {
		return nil, pc.errorf(
			node.Span(), CodeInvalidForeignKey,
			"foreign key references unknown column %q on table %q", refColumn, refTable,
		)
	}
	if !typesCompatible(srcType, targetCol.Type) {
		return nil, pc.errorf(
			node.Span(), CodeInvalidForeignKey,
			"foreign key column type %s is not compatible with referenced column %q (%s)",
			typeName(srcType), refColumn, typeName(targetCol.Type),
		)
	}
	return &catalog.ForeignKeyRef{ReferencedDB: db, ReferencedTable: refTable, ReferencedColumn: refColumn}, nil
}
