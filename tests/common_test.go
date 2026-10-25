package tests

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/rpc/storage_server"
	"github.com/makeshift-engineering/penguin-db/internal/sql/executor"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
	"github.com/makeshift-engineering/penguin-db/internal/wire"
)

// QueryResult captures execution results, row tuples, and error diagnostics returned for a single statement.
type QueryResult struct {
	CommandTag string
	Rows       [][]string
	Err        error
}

// setupIntegrationServer initializes a real LSM Storage Engine, a real gRPC StorageServer daemon,
// and a real RemoteKV store client, booting the wire.Server daemon over a TCP socket.
func setupIntegrationServer(t *testing.T) (net.Conn, string, func()) {
	t.Helper()

	tempDir := t.TempDir()

	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(tempDir, opts)
	if err != nil {
		t.Fatalf("failed to instantiate storage engine: %v", err)
	}

	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = engine.Close()
		t.Fatalf("failed to listen on ephemeral gRPC port: %v", err)
	}

	grpcServer := grpc.NewServer()
	storageServer := storage_server.NewStorageServer(engine)
	storagepb.RegisterStorageServiceServer(grpcServer, storageServer)

	go func() {
		_ = grpcServer.Serve(grpcLis)
	}()

	conn, err := grpc.NewClient(grpcLis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		_ = engine.Close()
		t.Fatalf("failed to dial gRPC storage server: %v", err)
	}

	grpcClient := storagepb.NewStorageServiceClient(conn)
	store := kv.NewRemoteKV(grpcClient)

	ctx := context.Background()
	cat, err := catalog.NewCatalog(ctx, store)
	if err != nil {
		cat = catalog.NewEmptyCatalog()
	}
	exec := executor.New(store, cat)

	pgLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = conn.Close()
		grpcServer.Stop()
		_ = engine.Close()
		t.Fatalf("failed to listen for pgwire connections: %v", err)
	}

	srv := wire.NewServer(pgLis.Addr().String(), exec, cat)
	srvCtx, cancelSrv := context.WithCancel(context.Background())
	go func() {
		_ = srv.Serve(srvCtx, pgLis)
	}()

	clientConn, err := net.Dial("tcp", pgLis.Addr().String())
	if err != nil {
		cancelSrv()
		_ = srv.Close()
		_ = conn.Close()
		grpcServer.Stop()
		_ = engine.Close()
		t.Fatalf("failed to dial wire server TCP socket: %v", err)
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = clientConn.Close()
			cancelSrv()
			_ = srv.Close()
			_ = conn.Close()
			grpcServer.Stop()
			storageServer.Stop()
			_ = engine.Close()
		})
	}
	t.Cleanup(cleanup)

	return clientConn, pgLis.Addr().String(), cleanup
}

// performHandshake performs SSL negotiation decline and initial protocol v3 startup handshake.
func performHandshake(t *testing.T, conn net.Conn) {
	t.Helper()

	sslReq := []byte{0, 0, 0, 8, 0x04, 0xD2, 0x16, 0x2F}
	if _, err := conn.Write(sslReq); err != nil {
		t.Fatalf("failed to transmit SSLRequest header: %v", err)
	}

	var sslResp [1]byte
	if _, err := io.ReadFull(conn, sslResp[:]); err != nil {
		t.Fatalf("failed to read SSL negotiation response: %v", err)
	}
	if sslResp[0] != 'N' {
		t.Fatalf("expected SSL response 'N', received %c", sslResp[0])
	}

	var payload bytes.Buffer
	var ver [4]byte
	binary.BigEndian.PutUint32(ver[:], wire.ProtocolVersion3)
	payload.Write(ver[:])
	payload.WriteString("user\x00penguin\x00database\x00testdb\x00\x00")

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(payload.Len()+4))
	conn.Write(lenBuf[:])
	conn.Write(payload.Bytes())

	for {
		var typeByte [1]byte
		if _, err := io.ReadFull(conn, typeByte[:]); err != nil {
			t.Fatalf("failed to read startup response message type: %v", err)
		}
		pLen, err := readInt32Helper(conn)
		if err != nil {
			t.Fatalf("failed to read startup packet length: %v", err)
		}
		pData := make([]byte, pLen-4)
		if _, err := io.ReadFull(conn, pData); err != nil {
			t.Fatalf("failed to read startup packet payload: %v", err)
		}

		if typeByte[0] == wire.MessageReadyForQuery {
			if pData[0] != 'I' {
				t.Fatalf("expected ReadyForQuery status 'I', received %c", pData[0])
			}
			break
		}
	}
}

// sendQuery formats a simple query wire packet, sends it across the TCP stream, and parses response frames.
func sendQuery(conn net.Conn, sql string) ([]QueryResult, error) {
	var qBuf bytes.Buffer
	qBuf.WriteByte(wire.MessageQuery)
	length := int32(len(sql) + 5)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(length))
	qBuf.Write(lenBuf[:])
	qBuf.WriteString(sql)
	qBuf.WriteByte(0)

	if _, err := conn.Write(qBuf.Bytes()); err != nil {
		return nil, err
	}

	var results []QueryResult
	var currentResult QueryResult

	for {
		var typeByte [1]byte
		if _, err := io.ReadFull(conn, typeByte[:]); err != nil {
			return nil, err
		}
		pLen, err := readInt32Helper(conn)
		if err != nil {
			return nil, err
		}
		pData := make([]byte, pLen-4)
		if _, err := io.ReadFull(conn, pData); err != nil {
			return nil, err
		}

		switch typeByte[0] {
		case wire.MessageCommandComplete:
			currentResult.CommandTag = string(bytes.TrimRight(pData, "\x00"))
			results = append(results, currentResult)
			currentResult = QueryResult{}

		case wire.MessageDataRow:
			r := bytes.NewReader(pData)
			var colCount int16
			binary.Read(r, binary.BigEndian, &colCount)
			row := make([]string, colCount)
			for i := 0; i < int(colCount); i++ {
				var valLen int32
				binary.Read(r, binary.BigEndian, &valLen)
				if valLen == -1 {
					row[i] = "NULL"
				} else {
					valBytes := make([]byte, valLen)
					r.Read(valBytes)
					row[i] = string(valBytes)
				}
			}
			currentResult.Rows = append(currentResult.Rows, row)

		case wire.MessageErrorResponse:
			currentResult.Err = fmt.Errorf("server error payload: %s", string(pData))

		case wire.MessageReadyForQuery:
			if currentResult.Err != nil || currentResult.CommandTag != "" {
				results = append(results, currentResult)
			}
			return results, nil
		}
	}
}

func readInt32Helper(r io.Reader) (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf[:])), nil
}

// SplitSQLStatements splits raw SQL content by semicolons while omitting comments and empty lines.
func SplitSQLStatements(content string) []string {
	var stmts []string
	rawStmts := strings.Split(content, ";")
	for _, raw := range rawStmts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		lines := strings.Split(stmt, "\n")
		var validLines []string
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			validLines = append(validLines, l)
		}
		cleanStmt := strings.TrimSpace(strings.Join(validLines, "\n"))
		if cleanStmt != "" {
			stmts = append(stmts, cleanStmt+";")
		}
	}
	return stmts
}

// loadAndExecuteFixture loads an SQL fixture file from testdata/sql and executes all setup statements against the server.
func loadAndExecuteFixture(t *testing.T, conn net.Conn, fixtureName string) {
	t.Helper()

	fixturePath := filepath.Join("testdata", "sql", fixtureName)
	contentBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read SQL fixture %s: %v", fixturePath, err)
	}

	stmts := SplitSQLStatements(string(contentBytes))
	for _, stmt := range stmts {
		query := stmt
		trimmed := strings.TrimSpace(query)
		if strings.HasPrefix(strings.ToLower(trimmed), "\\c ") || strings.HasPrefix(strings.ToLower(trimmed), "\\connect ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				db := strings.TrimRight(parts[1], ";")
				query = fmt.Sprintf("USE %s;", db)
			}
		}

		res, err := sendQuery(conn, query)
		if err != nil {
			t.Fatalf("fixture %s: network execution failed for statement %q: %v", fixtureName, query, err)
		}
		for _, r := range res {
			if r.Err != nil {
				t.Fatalf("fixture %s: SQL execution error for statement %q: %v", fixtureName, query, r.Err)
			}
		}
	}
}

// assertRowsEqual asserts that the actual query result rows match the expected result rows exactly,
// verifying row count, column count, and cell values in explicit order without sorting.
func assertRowsEqual(t *testing.T, actual, expected [][]string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("row count mismatch: got %d rows, expected %d rows\nactual rows:\n%+v\nexpected rows:\n%+v",
			len(actual), len(expected), actual, expected)
	}

	for r := range expected {
		if len(actual[r]) != len(expected[r]) {
			t.Fatalf("row %d column count mismatch: got %d columns, expected %d columns\ngot row: %+v\nexpected row: %+v",
				r, len(actual[r]), len(expected[r]), actual[r], expected[r])
		}
		for c := range expected[r] {
			if actual[r][c] != expected[r][c] {
				t.Fatalf("row %d column %d value mismatch: got %q, expected %q\ngot row: %+v\nexpected row: %+v",
					r, c, actual[r][c], expected[r][c], actual[r], expected[r])
			}
		}
	}
}
