package tests

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
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

	tempDir, err := os.MkdirTemp("", "penguin-db-integration-test-*")
	if err != nil {
		t.Fatalf("failed to create temporary storage directory: %v", err)
	}

	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(tempDir, opts)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to instantiate storage engine: %v", err)
	}

	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = engine.Close()
		os.RemoveAll(tempDir)
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
		os.RemoveAll(tempDir)
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
		conn.Close()
		grpcServer.Stop()
		_ = engine.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("failed to listen for pgwire connections: %v", err)
	}

	srv := wire.NewServer(pgLis.Addr().String(), exec, cat)
	srvCtx, cancelSrv := context.WithCancel(context.Background())
	go func() {
		_ = srv.Start(srvCtx)
	}()

	clientConn, err := net.Dial("tcp", pgLis.Addr().String())
	if err != nil {
		cancelSrv()
		_ = srv.Close()
		conn.Close()
		grpcServer.Stop()
		_ = engine.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("failed to dial wire server TCP socket: %v", err)
	}

	cleanup := func() {
		clientConn.Close()
		cancelSrv()
		_ = srv.Close()
		conn.Close()
		grpcServer.Stop()
		storageServer.Stop()
		_ = engine.Close()
		os.RemoveAll(tempDir)
	}

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
