// Package catalog implements PenguinDB's in-memory schema registry backed
// by the KV store. It provides two primary services:
//
//  1. KV-backed persistence — schema definitions live under system catalog
//     keys in the KV store, making them durable and eventually
//     Raft-replicated without any special mechanism.
//  2. In-memory cache — every node caches the catalog in a
//     [sync.RWMutex]-protected map for query-time validation without KV
//     round-trips.
//
// The catalog never writes to KV directly. DDL operations produce a list of
// [kv.Op] values that the caller (Row Store) submits through
// [kv.KV.WriteBatch]. After the batch commits, the caller invokes one of the
// Apply* methods to update the in-memory cache.
package catalog

import (
	"context"
	"fmt"
	"sync"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/encoding"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// Catalog is the in-memory schema registry. It caches database and table
// metadata in sync.RWMutex-protected maps for fast query-time lookups
// without KV round-trips.
//
// The Catalog never writes to KV directly. DDL operations produce
// []kv.Op lists via the Build*Ops functions, which the caller submits
// through kv.KV.WriteBatch. After the batch commits, the caller invokes
// the Apply* methods to update the in-memory cache.
type Catalog struct {
	mutex     sync.RWMutex
	databases map[string]*DatabaseMeta
	tables    map[string]map[string]*TableMeta // tables[db][table]
}

// NewCatalog creates a Catalog and bootstraps it by scanning all database
// and table metadata entries from the KV store. This is called once at
// node startup.
//
// If the KV store is empty, both scans return zero entries and the
// catalog starts empty.
func NewCatalog(ctx context.Context, store kv.KV) (*Catalog, error) {
	c := &Catalog{
		databases: make(map[string]*DatabaseMeta),
		tables:    make(map[string]map[string]*TableMeta),
	}

	// Load all database metadata.
	dbIter, err := store.Scan(ctx, encoding.CatalogDBScanPrefix())
	if err != nil {
		return nil, fmt.Errorf("catalog: scanning databases: %w", err)
	}
	defer dbIter.Close()

	for dbIter.Valid() {
		_, value := dbIter.Next()
		if value == nil {
			continue
		}

		meta, err := decodeDatabaseMeta(value)
		if err != nil {
			return nil, fmt.Errorf("catalog: decoding database meta: %w", err)
		}

		c.databases[meta.Name] = meta
		c.tables[meta.Name] = make(map[string]*TableMeta)
	}

	// Load all table metadata.
	tableIter, err := store.Scan(ctx, encoding.CatalogTableScanPrefix())
	if err != nil {
		return nil, fmt.Errorf("catalog: scanning tables: %w", err)
	}
	defer tableIter.Close()

	for tableIter.Valid() {
		_, value := tableIter.Next()
		if value == nil {
			continue
		}

		meta, err := decodeTableMeta(value)
		if err != nil {
			return nil, fmt.Errorf("catalog: decoding table meta: %w", err)
		}

		if c.tables[meta.Database] == nil {
			c.tables[meta.Database] = make(map[string]*TableMeta)
		}
		c.tables[meta.Database][meta.Name] = meta
	}

	return c, nil
}

// NewEmptyCatalog creates an empty Catalog without bootstrapping from a KV
// store. This is useful for testing and for scenarios where the KV store
// is not yet available.
func NewEmptyCatalog() *Catalog {
	return &Catalog{
		databases: make(map[string]*DatabaseMeta),
		tables:    make(map[string]map[string]*TableMeta),
	}
}

// DatabaseExists reports whether a database with the given name exists in
// the catalog. It acquires only a read lock and returns immediately.
func (catalog *Catalog) DatabaseExists(db string) bool {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	_, ok := catalog.databases[db]
	return ok
}

// GetDatabase returns the metadata for a database. Returns
// [ErrDatabaseNotFound] if no database with that name exists.
func (catalog *Catalog) GetDatabase(db string) (*DatabaseMeta, error) {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	meta, ok := catalog.databases[db]
	if !ok {
		return nil, ErrDatabaseNotFound
	}
	return meta, nil
}

// GetTable returns the metadata for a table in the given database.
// Returns [ErrTableNotFound] if the table does not exist.
func (catalog *Catalog) GetTable(db, table string) (*TableMeta, error) {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	dbTables, ok := catalog.tables[db]
	if !ok {
		return nil, ErrTableNotFound
	}
	meta, ok := dbTables[table]
	if !ok {
		return nil, ErrTableNotFound
	}
	return meta, nil
}

// ListTables returns all tables in the given database. Returns
// [ErrDatabaseNotFound] if the database does not exist.
func (catalog *Catalog) ListTables(db string) ([]*TableMeta, error) {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	if _, ok := catalog.databases[db]; !ok {
		return nil, ErrDatabaseNotFound
	}

	dbTables := catalog.tables[db]
	result := make([]*TableMeta, 0, len(dbTables))
	for _, meta := range dbTables {
		result = append(result, meta)
	}
	return result, nil
}

// ResolveColumn finds a non-dropped column by name in the given table.
// Returns [ErrTableNotFound] if the table does not exist, or
// [ErrColumnNotFound] if the column is not found or has been dropped.
func (catalog *Catalog) ResolveColumn(db, table, col string) (*ColumnMeta, error) {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	dbTables, ok := catalog.tables[db]
	if !ok {
		return nil, ErrTableNotFound
	}
	meta, ok := dbTables[table]
	if !ok {
		return nil, ErrTableNotFound
	}

	found := meta.FindColumn(col)
	if found == nil {
		return nil, ErrColumnNotFound
	}
	return found, nil
}

// PKColumnTypes returns the ordered list of data type kinds for the
// table's primary key columns. Returns [ErrTableNotFound] if the table
// does not exist.
func (catalog *Catalog) PKColumnTypes(db, table string) ([]ast.DataTypeKind, error) {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	dbTables, ok := catalog.tables[db]
	if !ok {
		return nil, ErrTableNotFound
	}
	meta, ok := dbTables[table]
	if !ok {
		return nil, ErrTableNotFound
	}

	if meta.HasSnowflakeID {
		return []ast.DataTypeKind{ast.TypeBigInt}, nil
	}

	types := make([]ast.DataTypeKind, 0, len(meta.PrimaryKey))
	for _, pkName := range meta.PrimaryKey {
		for _, col := range meta.Columns {
			if col.Name == pkName && !col.Dropped {
				types = append(types, col.Type)
				break
			}
		}
	}

	return types, nil
}

// ListDatabases returns all databases in the catalog.
func (catalog *Catalog) ListDatabases() []*DatabaseMeta {
	catalog.mutex.RLock()
	defer catalog.mutex.RUnlock()

	result := make([]*DatabaseMeta, 0, len(catalog.databases))
	for _, meta := range catalog.databases {
		result = append(result, meta)
	}
	return result
}
