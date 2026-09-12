package tests

import (
	"testing"
)

// TestSQLDDL_DatabaseLifecycle verifies CREATE DATABASE, USE DATABASE, and DROP DATABASE statement execution over TCP.
func TestSQLDDL_DatabaseLifecycle(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	createDbRes, err := sendQuery(conn, "CREATE DATABASE ddl_test_db;")
	if err != nil || len(createDbRes) == 0 || createDbRes[0].Err != nil {
		t.Fatalf("CREATE DATABASE failed: err=%v, res=%+v", err, createDbRes)
	}
	if createDbRes[0].CommandTag != "CREATE DATABASE" {
		t.Fatalf("expected command tag 'CREATE DATABASE', received %q", createDbRes[0].CommandTag)
	}

	useDbRes, err := sendQuery(conn, "USE ddl_test_db;")
	if err != nil || len(useDbRes) == 0 || useDbRes[0].Err != nil {
		t.Fatalf("USE DATABASE failed: err=%v, res=%+v", err, useDbRes)
	}

	dropDbRes, err := sendQuery(conn, "DROP DATABASE ddl_test_db;")
	if err != nil || len(dropDbRes) == 0 || dropDbRes[0].Err != nil {
		t.Fatalf("DROP DATABASE failed: err=%v, res=%+v", err, dropDbRes)
	}
}

// TestSQLDDL_TableLifecycle verifies CREATE TABLE and DROP TABLE schema management over wire protocol.
func TestSQLDDL_TableLifecycle(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	_, _ = sendQuery(conn, "CREATE DATABASE table_ddl_db; USE table_ddl_db;")

	createTableSQL := "CREATE TABLE accounts (account_id INT PRIMARY KEY, holder_name VARCHAR(100), balance DOUBLE);"
	createTableRes, err := sendQuery(conn, createTableSQL)
	if err != nil || len(createTableRes) == 0 || createTableRes[0].Err != nil {
		t.Fatalf("CREATE TABLE failed: err=%v, res=%+v", err, createTableRes)
	}
	if createTableRes[0].CommandTag != "CREATE TABLE" {
		t.Fatalf("expected command tag 'CREATE TABLE', received %q", createTableRes[0].CommandTag)
	}

	dropTableRes, err := sendQuery(conn, "DROP TABLE accounts;")
	if err != nil || len(dropTableRes) == 0 || dropTableRes[0].Err != nil {
		t.Fatalf("DROP TABLE failed: err=%v, res=%+v", err, dropTableRes)
	}
}
