package catalog

import "errors"

// Sentinel errors returned by the catalog package.
var (
	// ErrDatabaseExists is returned when attempting to create a database
	// that already exists in the catalog and IF NOT EXISTS was not specified.
	ErrDatabaseExists = errors.New("catalog: database already exists")

	// ErrDatabaseNotFound is returned when a referenced database does not
	// exist in the catalog.
	ErrDatabaseNotFound = errors.New("catalog: database not found")

	// ErrTableExists is returned when attempting to create a table that
	// already exists in the catalog and IF NOT EXISTS was not specified.
	ErrTableExists = errors.New("catalog: table already exists")

	// ErrTableNotFound is returned when a referenced table does not exist
	// in the catalog.
	ErrTableNotFound = errors.New("catalog: table not found")

	// ErrColumnNotFound is returned when a referenced column does not
	// exist (or has been dropped) in the table's schema.
	ErrColumnNotFound = errors.New("catalog: column not found")

	// ErrDuplicateColumn is returned when a CREATE TABLE or ALTER TABLE ADD
	// would introduce two columns with the same name.
	ErrDuplicateColumn = errors.New("catalog: duplicate column name")

	// ErrCannotDropPKColumn is returned when attempting to drop a column
	// that is part of the table's primary key.
	ErrCannotDropPKColumn = errors.New("catalog: cannot drop primary key column")

	// ErrUnsupportedAlter is returned for ALTER TABLE operations that are
	// not supported in v1 (e.g. change column type, add/remove PK columns,
	// add NOT NULL column without DEFAULT).
	ErrUnsupportedAlter = errors.New("catalog: alter operation not supported in v1")

	// ErrSchemaViolation is returned when a DDL operation would violate a
	// structural schema constraint (e.g. PK column names not in column list).
	ErrSchemaViolation = errors.New("catalog: schema constraint violated")

	// ErrCatalogDBMissing is returned when a table operation references a
	// database that does not exist in the catalog.
	ErrCatalogDBMissing = errors.New("catalog: database does not exist for table operation")

	// ErrPKColumnNotFound is returned during table creation when a column
	// name listed in the PRIMARY KEY constraint does not match any column
	// definition.
	ErrPKColumnNotFound = errors.New("catalog: primary key column not found in column definitions")
)
