package catalog

import (
	"fmt"
	"slices"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/encoding"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
)

// BuildCreateDatabaseOps constructs the KV operations needed to persist a
// new database in the catalog. It returns a single OpPut for the database
// metadata key.
//
// The caller is responsible for checking whether the database already
// exists before submitting the returned ops.
func BuildCreateDatabaseOps(meta *DatabaseMeta) ([]kv.Op, error) {
	key, err := encoding.EncodeCatalogDBKey(meta.Name)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding db key: %w", err)
	}

	value, err := encodeDatabaseMeta(meta)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding db meta: %w", err)
	}

	return []kv.Op{
		{Type: kv.OpPut, Key: key, Value: value},
	}, nil
}

// BuildDropDatabaseOps constructs the KV operations needed to remove a
// database and all its table schema entries from the catalog.
func BuildDropDatabaseOps(db string, tables []*TableMeta) ([]kv.Op, error) {
	dbKey, err := encoding.EncodeCatalogDBKey(db)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding db key: %w", err)
	}

	ops := make([]kv.Op, 0, 1+len(tables)*2)
	ops = append(ops, kv.Op{Type: kv.OpDelete, Key: dbKey})

	for _, t := range tables {
		tableKey, err := encoding.EncodeCatalogTableKey(db, t.Name)
		if err != nil {
			return nil, fmt.Errorf("catalog: encoding table key: %w", err)
		}
		ops = append(ops, kv.Op{Type: kv.OpDelete, Key: tableKey})

		seqKey, err := encoding.EncodeCatalogSeqKey(db, t.Name)
		if err != nil {
			return nil, fmt.Errorf("catalog: encoding seq key: %w", err)
		}
		ops = append(ops, kv.Op{Type: kv.OpDelete, Key: seqKey})
	}

	return ops, nil
}

// BuildCreateTableOps validates the table metadata against the catalog and
// constructs the KV operations needed to persist a new table schema.
//
// Pre-write validation performed:
//   - Database must exist in the catalog.
//   - No table with the same name may exist.
//   - Column names must be unique.
//   - PK column names must all appear in the column definitions.
func BuildCreateTableOps(catalog *Catalog, meta *TableMeta) ([]kv.Op, error) {
	// Validate database exists.
	catalog.mutex.RLock()
	_, dbExists := catalog.databases[meta.Database]
	_, tableExists := catalog.tables[meta.Database][meta.Name]
	catalog.mutex.RUnlock()

	if !dbExists {
		return nil, ErrCatalogDBMissing
	}
	if tableExists {
		return nil, ErrTableExists
	}

	// Validate column names are unique.
	seen := make(map[string]bool, len(meta.Columns))
	for _, col := range meta.Columns {
		if seen[col.Name] {
			return nil, ErrDuplicateColumn
		}
		seen[col.Name] = true
	}

	// Validate PK column names exist in the column list.
	for _, pkName := range meta.PrimaryKey {
		if !seen[pkName] {
			return nil, ErrPKColumnNotFound
		}
	}

	// Build the KV ops.
	tableKey, err := encoding.EncodeCatalogTableKey(meta.Database, meta.Name)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table key: %w", err)
	}

	value, err := encodeTableMeta(meta)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table meta: %w", err)
	}

	ops := []kv.Op{
		{Type: kv.OpPut, Key: tableKey, Value: value},
	}

	return ops, nil
}

// BuildDropTableOps constructs the KV operations needed to remove a table
// schema and its sequence counter from the catalog.
func BuildDropTableOps(db, table string) ([]kv.Op, error) {
	tableKey, err := encoding.EncodeCatalogTableKey(db, table)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table key: %w", err)
	}

	seqKey, err := encoding.EncodeCatalogSeqKey(db, table)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding seq key: %w", err)
	}

	return []kv.Op{
		{Type: kv.OpDelete, Key: tableKey},
		{Type: kv.OpDelete, Key: seqKey},
	}, nil
}

// BuildAlterTableOps validates the schema transition from oldMeta to
// newMeta and constructs the KV operations to persist the change. The
// newMeta.Version is set to oldMeta.Version + 1 on success.
func BuildAlterTableOps(oldMeta, newMeta *TableMeta) ([]kv.Op, error) {
	if err := validateAlter(oldMeta, newMeta); err != nil {
		return nil, err
	}

	newMeta.Version = oldMeta.Version + 1

	tableKey, err := encoding.EncodeCatalogTableKey(newMeta.Database, newMeta.Name)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table key: %w", err)
	}

	value, err := encodeTableMeta(newMeta)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table meta: %w", err)
	}

	return []kv.Op{
		{Type: kv.OpPut, Key: tableKey, Value: value},
	}, nil
}

// BuildRenameTableOps constructs the KV operations to atomically rename a
// table by deleting the old catalog key and inserting a new one.
func BuildRenameTableOps(db, oldName, newName string, meta *TableMeta) ([]kv.Op, error) {
	oldKey, err := encoding.EncodeCatalogTableKey(db, oldName)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding old table key: %w", err)
	}

	meta.Name = newName

	newKey, err := encoding.EncodeCatalogTableKey(db, newName)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding new table key: %w", err)
	}

	value, err := encodeTableMeta(meta)
	if err != nil {
		return nil, fmt.Errorf("catalog: encoding table meta: %w", err)
	}

	return []kv.Op{
		{Type: kv.OpDelete, Key: oldKey},
		{Type: kv.OpPut, Key: newKey, Value: value},
	}, nil
}

// validateAlter checks that the transition from oldMeta to newMeta is a
// legal ALTER TABLE operation.
func validateAlter(oldMeta, newMeta *TableMeta) error {
	// PK columns must not change.
	if len(oldMeta.PrimaryKey) != len(newMeta.PrimaryKey) {
		return ErrUnsupportedAlter
	}
	for i := range oldMeta.PrimaryKey {
		if oldMeta.PrimaryKey[i] != newMeta.PrimaryKey[i] {
			return ErrUnsupportedAlter
		}
	}

	// Walk through columns to detect the kind of change.
	oldActive := oldMeta.ActiveColumns()
	newAll := newMeta.Columns

	// Build a map of old active column names for quick lookup.
	oldColMap := make(map[string]*ColumnMeta, len(oldActive))
	for i := range oldActive {
		oldColMap[oldActive[i].Name] = &oldActive[i]
	}

	for i := range newAll {
		col := &newAll[i]

		if col.Dropped {
			// Check: cannot drop a PK column.
			if slices.Contains(oldMeta.PrimaryKey, col.Name) {
				return ErrCannotDropPKColumn
			}
			continue
		}

		oldCol, existed := oldColMap[col.Name]
		if !existed {
			// This is a newly added column.
			if col.NotNull && col.DefaultValue == nil {
				return ErrUnsupportedAlter
			}
			continue
		}

		// Column type changes are not supported.
		if oldCol.Type != col.Type {
			return ErrUnsupportedAlter
		}
	}

	return nil
}
