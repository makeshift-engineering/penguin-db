package catalog

// ApplyCreateDatabase updates the in-memory cache to reflect a newly
// created database. This must be called only after the corresponding KV
// write has committed.
func (catalog *Catalog) ApplyCreateDatabase(meta *DatabaseMeta) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.databases[meta.Name] = meta
	if catalog.tables[meta.Name] == nil {
		catalog.tables[meta.Name] = make(map[string]*TableMeta)
	}
}

// ApplyDropDatabase removes a database and all its tables from the
// in-memory cache. This must be called only after the corresponding KV
// write has committed.
func (catalog *Catalog) ApplyDropDatabase(db string) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	delete(catalog.databases, db)
	delete(catalog.tables, db)
}

// ApplyCreateTable inserts a new table into the in-memory cache. This must
// be called only after the corresponding KV write has committed.
func (catalog *Catalog) ApplyCreateTable(meta *TableMeta) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.tables[meta.Database] == nil {
		catalog.tables[meta.Database] = make(map[string]*TableMeta)
	}
	catalog.tables[meta.Database][meta.Name] = meta
}

// ApplyDropTable removes a table from the in-memory cache. This must be
// called only after the corresponding KV write has committed.
func (catalog *Catalog) ApplyDropTable(db, table string) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if dbTables, ok := catalog.tables[db]; ok {
		delete(dbTables, table)
	}
}

// ApplyAlterTable replaces the existing table entry in the in-memory cache
// with the new metadata. The Version field should already be incremented
// by [BuildAlterTableOps]. This must be called only after the
// corresponding KV write has committed.
func (catalog *Catalog) ApplyAlterTable(newMeta *TableMeta) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.tables[newMeta.Database] == nil {
		catalog.tables[newMeta.Database] = make(map[string]*TableMeta)
	}
	catalog.tables[newMeta.Database][newMeta.Name] = newMeta
}

// ApplyRenameTable atomically removes the old table name and inserts the
// new one in the in-memory cache. This must be called only after the
// corresponding KV write has committed.
func (catalog *Catalog) ApplyRenameTable(db, oldName, newName string, meta *TableMeta) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if dbTables, ok := catalog.tables[db]; ok {
		delete(dbTables, oldName)
	}
	if catalog.tables[db] == nil {
		catalog.tables[db] = make(map[string]*TableMeta)
	}
	catalog.tables[db][newName] = meta
}
