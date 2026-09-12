package tests

import (
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/wire"
)

// TestSQLErrorRecovery_SyntaxAndPlanningErrors verifies wire server error response formatting and TCP connection recovery after invalid SQL statements.
func TestSQLErrorRecovery_SyntaxAndPlanningErrors(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	syntaxErrRes, err := sendQuery(conn, "MALFORMED INVALID SQL STATEMENT;")
	if err != nil {
		t.Fatalf("unexpected network transport error on syntax failure: %v", err)
	}
	if len(syntaxErrRes) == 0 || syntaxErrRes[0].Err == nil {
		t.Fatalf("expected error payload for malformed SQL syntax, received success: %+v", syntaxErrRes)
	}

	recoverRes, err := sendQuery(conn, "CREATE DATABASE recovery_db; USE recovery_db;")
	if err != nil || len(recoverRes) != 2 || recoverRes[0].Err != nil {
		t.Fatalf("failed to recover connection after syntax error: err=%v, res=%+v", err, recoverRes)
	}

	planningErrRes, err := sendQuery(conn, "SELECT * FROM non_existent_table_reference;")
	if err != nil {
		t.Fatalf("unexpected network transport error on planning failure: %v", err)
	}
	if len(planningErrRes) == 0 || planningErrRes[0].Err == nil {
		t.Fatalf("expected planning error payload for non-existent table, received success: %+v", planningErrRes)
	}

	validRes, err := sendQuery(conn, "CREATE TABLE test_table (id INT PRIMARY KEY); INSERT INTO test_table VALUES (10);")
	if err != nil || len(validRes) != 2 || validRes[0].Err != nil {
		t.Fatalf("failed to execute queries after planning error recovery: err=%v, res=%+v", err, validRes)
	}
}

// TestSQLProtocol_TerminateConnection tests frontend MsgTerminate packet parsing and connection disconnection by the server.
func TestSQLProtocol_TerminateConnection(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	terminateFrame := []byte{wire.MessageTerminate, 0, 0, 0, 4}
	if _, err := conn.Write(terminateFrame); err != nil {
		t.Fatalf("failed to transmit Terminate message frame: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	var dummyBuf [1]byte
	_, err := conn.Read(dummyBuf[:])
	if err == nil {
		t.Fatalf("expected TCP socket connection to be closed by server, but read succeeded: %v", dummyBuf)
	}
}
