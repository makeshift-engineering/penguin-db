package tests

import (
	"strings"
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

// TestSQLErrorReporting_DetailedDiagnostics tests that error responses over the wire contain
// rich compiler-style diagnostics: standardized error codes [E####], SQLSTATE codes, source
// line snippets with carets in Detail, and character Position offsets.
func TestSQLErrorReporting_DetailedDiagnostics(t *testing.T) {
	conn, _, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, conn)

	// 1. Syntax Error: malformed statement start
	res, err := sendQuery(conn, "MALFORMED STATEMENT;")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(res) == 0 || res[0].Err == nil {
		t.Fatalf("expected error for syntax mistake, got %+v", res)
	}
	errMsg := res[0].Err.Error()
	t.Logf("\n[Syntax Error Response]:\n%s\n", errMsg)
	// Must contain diagnostic code [E2009] (CodeMalformedStatement)
	if !strings.Contains(errMsg, "[E2009]") {
		t.Errorf("expected [E2009] in syntax error, got: %s", errMsg)
	}
	// Must contain line:col pointing to error
	if !strings.Contains(errMsg, "1:1:") {
		t.Errorf("expected '1:1:' position in syntax error, got: %s", errMsg)
	}
	// Must contain SQLSTATE 42601
	if !strings.Contains(errMsg, "42601") {
		t.Errorf("expected SQLSTATE 42601 in syntax error, got: %s", errMsg)
	}
	// Must contain caret snippet
	if !strings.Contains(errMsg, "^") {
		t.Errorf("expected caret pointer in syntax error, got: %s", errMsg)
	}

	// 2. Planning Error: table not found in existing database
	_, err = sendQuery(conn, "CREATE DATABASE err_db; USE err_db;")
	if err != nil {
		t.Fatalf("setup database failed: %v", err)
	}
	res, err = sendQuery(conn, "SELECT * FROM non_existent_table;")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(res) == 0 || res[0].Err == nil {
		t.Fatalf("expected error for non-existent table, got %+v", res)
	}
	errMsg = res[0].Err.Error()
	t.Logf("\n[Planning Error Response]:\n%s\n", errMsg)
	if !strings.Contains(errMsg, "[E3003]") {
		t.Errorf("expected [E3003] in planning error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "42P01") {
		t.Errorf("expected SQLSTATE 42P01 in planning error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "^") {
		t.Errorf("expected caret pointer in planning error, got: %s", errMsg)
	}

	// 3. Execution Error: duplicate primary key
	_, err = sendQuery(conn, "CREATE TABLE users (id INT PRIMARY KEY); INSERT INTO users VALUES (1);")
	if err != nil {
		t.Fatalf("setup table failed: %v", err)
	}
	dupRes, err := sendQuery(conn, "INSERT INTO users VALUES (1);")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(dupRes) == 0 || dupRes[0].Err == nil {
		t.Fatalf("expected error on duplicate PK insert, got %+v", dupRes)
	}
	errMsg = dupRes[0].Err.Error()
	t.Logf("\n[Execution Error Response]:\n%s\n", errMsg)
	if !strings.Contains(errMsg, "[E4003]") {
		t.Errorf("expected [E4003] in duplicate PK error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "23505") {
		t.Errorf("expected SQLSTATE 23505 in duplicate PK error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "^") {
		t.Errorf("expected caret pointer in duplicate PK error, got: %s", errMsg)
	}
	if strings.Contains(errMsg, "executor:") || strings.Contains(errMsg, "rpc error") {
		t.Errorf("internal executor/gRPC prefixes should not leak: %s", errMsg)
	}

	// 4. Execution Error: division by zero
	divRes, err := sendQuery(conn, "SELECT 10 / 0;")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(divRes) == 0 || divRes[0].Err == nil {
		t.Fatalf("expected error on division by zero, got %+v", divRes)
	}
	errMsg = divRes[0].Err.Error()
	t.Logf("\n[Division By Zero Error Response]:\n%s\n", errMsg)
	if !strings.Contains(errMsg, "[E4005]") {
		t.Errorf("expected [E4005] in division by zero error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "22012") {
		t.Errorf("expected SQLSTATE 22012 in division by zero error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "^") {
		t.Errorf("expected caret pointer in division by zero error, got: %s", errMsg)
	}
	if strings.Contains(errMsg, "executor:") {
		t.Errorf("internal executor prefix should not leak: %s", errMsg)
	}

	// 5. Execution Error: NOT NULL violation
	_, err = sendQuery(conn, "CREATE TABLE nn_test (id INT PRIMARY KEY, name TEXT NOT NULL);")
	if err != nil {
		t.Fatalf("setup table failed: %v", err)
	}
	nnRes, err := sendQuery(conn, "INSERT INTO nn_test VALUES (1, NULL);")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(nnRes) == 0 || nnRes[0].Err == nil {
		t.Fatalf("expected error on NOT NULL violation, got %+v", nnRes)
	}
	errMsg = nnRes[0].Err.Error()
	t.Logf("\n[NOT NULL Violation Response]:\n%s\n", errMsg)
	if !strings.Contains(errMsg, "[E4002]") {
		t.Errorf("expected [E4002] in NOT NULL violation error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "23502") {
		t.Errorf("expected SQLSTATE 23502 in NOT NULL violation error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "^") {
		t.Errorf("expected caret pointer in NOT NULL violation error, got: %s", errMsg)
	}
	if strings.Contains(errMsg, "executor:") {
		t.Errorf("internal executor prefix should not leak: %s", errMsg)
	}

	// 6. Execution Error: Omitted NOT NULL column (statement-level span fallback)
	omitRes, err := sendQuery(conn, "INSERT INTO nn_test (id) VALUES (2);")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(omitRes) == 0 || omitRes[0].Err == nil {
		t.Fatalf("expected error on omitted NOT NULL column, got %+v", omitRes)
	}
	omitMsg := omitRes[0].Err.Error()
	t.Logf("\n[Omitted NOT NULL Column Response]:\n%s\n", omitMsg)
	if !strings.Contains(omitMsg, "[E4002]") {
		t.Errorf("expected [E4002] in omitted NOT NULL column error, got: %s", omitMsg)
	}
	if !strings.Contains(omitMsg, "23502") {
		t.Errorf("expected SQLSTATE 23502 in omitted NOT NULL column error, got: %s", omitMsg)
	}
	if !strings.Contains(omitMsg, "^") {
		t.Errorf("expected caret pointer in omitted NOT NULL column error, got: %s", omitMsg)
	}

	// 7. Execution Error: UPDATE duplicate primary key
	_, err = sendQuery(conn, "INSERT INTO users VALUES (2);")
	if err != nil {
		t.Fatalf("setup user row failed: %v", err)
	}
	updRes, err := sendQuery(conn, "UPDATE users SET id = 1 WHERE id = 2;")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(updRes) == 0 || updRes[0].Err == nil {
		t.Fatalf("expected error on UPDATE duplicate PK, got %+v", updRes)
	}
	updMsg := updRes[0].Err.Error()
	t.Logf("\n[UPDATE Duplicate PK Response]:\n%s\n", updMsg)
	if !strings.Contains(updMsg, "[E4003]") {
		t.Errorf("expected [E4003] in UPDATE duplicate PK error, got: %s", updMsg)
	}
	if !strings.Contains(updMsg, "23505") {
		t.Errorf("expected SQLSTATE 23505 in UPDATE duplicate PK error, got: %s", updMsg)
	}
	if !strings.Contains(updMsg, "^") {
		t.Errorf("expected caret pointer in UPDATE duplicate PK error, got: %s", updMsg)
	}

	// 8. Execution Error: VARCHAR length exceeded
	_, err = sendQuery(conn, "CREATE TABLE vc_test (id INT PRIMARY KEY, tag VARCHAR(4));")
	if err != nil {
		t.Fatalf("setup table failed: %v", err)
	}
	vcRes, err := sendQuery(conn, "INSERT INTO vc_test VALUES (1, 'TOO_LONG_STRING');")
	if err != nil {
		t.Fatalf("unexpected network error: %v", err)
	}
	if len(vcRes) == 0 || vcRes[0].Err == nil {
		t.Fatalf("expected error on VARCHAR too long, got %+v", vcRes)
	}
	vcMsg := vcRes[0].Err.Error()
	t.Logf("\n[VARCHAR Too Long Response]:\n%s\n", vcMsg)
	if !strings.Contains(vcMsg, "[E4007]") {
		t.Errorf("expected [E4007] in VARCHAR too long error, got: %s", vcMsg)
	}
	if !strings.Contains(vcMsg, "22001") {
		t.Errorf("expected SQLSTATE 22001 in VARCHAR too long error, got: %s", vcMsg)
	}
	if !strings.Contains(vcMsg, "^") {
		t.Errorf("expected caret pointer in VARCHAR too long error, got: %s", vcMsg)
	}
}
