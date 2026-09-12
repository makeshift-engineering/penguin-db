package tests

import (
	"testing"
)

// TestSQLMultiStatement_BatchExecution tests executing multiple SQL statements contained within a single Simple Query TCP message payload.
func TestSQLMultiStatement_BatchExecution(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	setupBatchSQL := "CREATE DATABASE multi_test_db; USE multi_test_db;"
	setupRes, err := sendQuery(conn, setupBatchSQL)
	if err != nil || len(setupRes) != 2 {
		t.Fatalf("multi-statement database setup failed: err=%v, resCount=%d", err, len(setupRes))
	}
	if setupRes[0].CommandTag != "CREATE DATABASE" {
		t.Fatalf("unexpected command tag for statement 0: %s", setupRes[0].CommandTag)
	}

	execBatchSQL := "CREATE TABLE orders (order_id INT PRIMARY KEY); INSERT INTO orders VALUES (5001), (5002); SELECT order_id FROM orders ORDER BY order_id ASC;"
	execRes, err := sendQuery(conn, execBatchSQL)
	if err != nil || len(execRes) != 3 {
		t.Fatalf("multi-statement query batch execution failed: err=%v, resCount=%d", err, len(execRes))
	}

	if execRes[0].CommandTag != "CREATE TABLE" {
		t.Fatalf("unexpected batch command tag 0: %s", execRes[0].CommandTag)
	}
	if execRes[1].CommandTag != "INSERT 0 2" {
		t.Fatalf("unexpected batch command tag 1: %s", execRes[1].CommandTag)
	}
	if execRes[2].CommandTag != "SELECT 2" || len(execRes[2].Rows) != 2 {
		t.Fatalf("unexpected batch command tag 2 / rows: tag=%s, rows=%+v", execRes[2].CommandTag, execRes[2].Rows)
	}
	if execRes[2].Rows[0][0] != "5001" || execRes[2].Rows[1][0] != "5002" {
		t.Fatalf("unexpected batch tuple values: %+v", execRes[2].Rows)
	}
}
