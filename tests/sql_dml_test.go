package tests

import (
	"testing"
)

// TestSQLDML_InsertSelect tests single and multi-row INSERT statement execution and SELECT query tuple retrieval.
func TestSQLDML_InsertSelect(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	_, _ = sendQuery(conn, "CREATE DATABASE dml_db; USE dml_db;")
	_, _ = sendQuery(conn, "CREATE TABLE employees (id INT PRIMARY KEY, name VARCHAR(50), department VARCHAR(50));")

	insertSingleRes, err := sendQuery(conn, "INSERT INTO employees VALUES (1, 'Alice', 'Engineering');")
	if err != nil || len(insertSingleRes) == 0 || insertSingleRes[0].Err != nil {
		t.Fatalf("single row INSERT failed: err=%v, res=%+v", err, insertSingleRes)
	}
	if insertSingleRes[0].CommandTag != "INSERT 0 1" {
		t.Fatalf("expected command tag 'INSERT 0 1', received %q", insertSingleRes[0].CommandTag)
	}

	insertBatchRes, err := sendQuery(conn, "INSERT INTO employees VALUES (2, 'Bob', 'Sales'), (3, 'Charlie', 'Engineering');")
	if err != nil || len(insertBatchRes) == 0 || insertBatchRes[0].Err != nil {
		t.Fatalf("batch INSERT failed: err=%v, res=%+v", err, insertBatchRes)
	}
	if insertBatchRes[0].CommandTag != "INSERT 0 2" {
		t.Fatalf("expected command tag 'INSERT 0 2', received %q", insertBatchRes[0].CommandTag)
	}

	selectRes, err := sendQuery(conn, "SELECT id, name, department FROM employees ORDER BY id ASC;")
	if err != nil || len(selectRes) == 0 || selectRes[0].Err != nil {
		t.Fatalf("SELECT failed: err=%v, res=%+v", err, selectRes)
	}
	if selectRes[0].CommandTag != "SELECT 3" || len(selectRes[0].Rows) != 3 {
		t.Fatalf("unexpected SELECT results: tag=%s, rows=%+v", selectRes[0].CommandTag, selectRes[0].Rows)
	}
	if selectRes[0].Rows[0][1] != "Alice" || selectRes[0].Rows[1][1] != "Bob" || selectRes[0].Rows[2][1] != "Charlie" {
		t.Fatalf("unexpected row contents: %+v", selectRes[0].Rows)
	}
}

// TestSQLDML_UpdateDelete tests UPDATE statement value mutation and DELETE statement record eviction.
func TestSQLDML_UpdateDelete(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	_, _ = sendQuery(conn, "CREATE DATABASE update_db; USE update_db;")
	_, _ = sendQuery(conn, "CREATE TABLE inventory (sku INT PRIMARY KEY, qty INT);")
	_, _ = sendQuery(conn, "INSERT INTO inventory VALUES (101, 50), (102, 100);")

	updateRes, err := sendQuery(conn, "UPDATE inventory SET qty = 75 WHERE sku = 101;")
	if err != nil || len(updateRes) == 0 || updateRes[0].Err != nil {
		t.Fatalf("UPDATE statement failed: err=%v, res=%+v", err, updateRes)
	}
	if updateRes[0].CommandTag != "UPDATE 1" {
		t.Fatalf("expected command tag 'UPDATE 1', received %q", updateRes[0].CommandTag)
	}

	deleteRes, err := sendQuery(conn, "DELETE FROM inventory WHERE sku = 102;")
	if err != nil || len(deleteRes) == 0 || deleteRes[0].Err != nil {
		t.Fatalf("DELETE statement failed: err=%v, res=%+v", err, deleteRes)
	}
	if deleteRes[0].CommandTag != "DELETE 1" {
		t.Fatalf("expected command tag 'DELETE 1', received %q", deleteRes[0].CommandTag)
	}

	verifyRes, err := sendQuery(conn, "SELECT sku, qty FROM inventory;")
	if err != nil || len(verifyRes) == 0 || verifyRes[0].Err != nil {
		t.Fatalf("verification SELECT failed: err=%v, res=%+v", err, verifyRes)
	}
	if len(verifyRes[0].Rows) != 1 || verifyRes[0].Rows[0][0] != "101" || verifyRes[0].Rows[0][1] != "75" {
		t.Fatalf("unexpected remaining records: %+v", verifyRes[0].Rows)
	}
}
