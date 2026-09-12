package wire

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/executor"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
	"github.com/makeshift-engineering/penguin-db/internal/sql/parser"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// Session encapsulates connection-scoped state for an active client connection.
type Session struct {
	User           string
	ActiveDatabase string
}

// Server manages TCP listener lifecycle and PostgreSQL wire protocol client connections.
type Server struct {
	addr     string
	exec     *executor.Executor
	catalog  *catalog.Catalog
	listener net.Listener
}

// NewServer instantiates a wire Server backed by an executor engine and catalog.
func NewServer(addr string, exec *executor.Executor, cat *catalog.Catalog) *Server {
	return &Server{
		addr:    addr,
		exec:    exec,
		catalog: cat,
	}
}

// Start opens a TCP socket and accepts client connections until context cancellation.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("penguinwire: failed to listen on %s: %w", s.addr, err)
	}
	s.listener = l

	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			continue
		}
		go s.handleConnection(ctx, conn)
	}
}

// Close gracefully terminates the underlying TCP network listener.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// handleConnection manages connection handshake negotiation and processes incoming query packets.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	session := &Session{ActiveDatabase: "testdb"}
	if err := s.performHandshake(conn, session); err != nil {
		return
	}

	for {
		var typeBuf [1]byte
		if _, err := io.ReadFull(conn, typeBuf[:]); err != nil {
			return
		}

		switch typeBuf[0] {
		case MessageQuery:
			length, err := readInt32(conn)
			if err != nil {
				return
			}

			sqlPayload := make([]byte, length-4)
			if _, err := io.ReadFull(conn, sqlPayload); err != nil {
				return
			}
			query := string(bytes.TrimRight(sqlPayload, "\x00"))
			s.executeQuery(ctx, conn, session, query)

		case MessageTerminate:
			return

		default:
			length, err := readInt32(conn)
			if err != nil {
				return
			}
			io.CopyN(io.Discard, conn, int64(length-4))
			conn.Write(encodeErrorResponse("ERROR", "08004", "unsupported frontend message type"))
			conn.Write(encodeReadyForQuery('I'))
		}
	}
}

// performHandshake negotiates SSL decline and processes frontend startup parameters.
func (s *Server) performHandshake(conn net.Conn, session *Session) error {
	for {
		length, err := readInt32(conn)
		if err != nil {
			return err
		}

		versionOrCode, err := readInt32(conn)
		if err != nil {
			return err
		}

		if versionOrCode == SSLRequestCode {
			if _, err := conn.Write([]byte{'N'}); err != nil {
				return err
			}
			continue
		}

		if versionOrCode == ProtocolVersion3 {
			paramBytes := make([]byte, length-8)
			if _, err := io.ReadFull(conn, paramBytes); err != nil {
				return err
			}

			r := bytes.NewReader(paramBytes)

			for r.Len() > 0 {
				k, err := readCString(r)
				if err != nil || k == "" {
					break
				}
				v, err := readCString(r)
				if err != nil {
					break
				}
				if k == "user" && v != "" {
					session.User = v
				}
				if k == "database" && v != "" {
					session.ActiveDatabase = v
				}
			}

			conn.Write(encodeAuthOk())
			conn.Write(encodeParameterStatus("server_version", "15.0 (PenguinDB)"))
			conn.Write(encodeParameterStatus("client_encoding", "UTF8"))
			conn.Write(encodeParameterStatus("server_encoding", "UTF8"))
			conn.Write(encodeParameterStatus("DateStyle", "ISO, MDY"))
			conn.Write(encodeParameterStatus("integer_datetimes", "on"))
			conn.Write(encodeParameterStatus("is_superuser", "on"))
			if session.User != "" {
				conn.Write(encodeParameterStatus("session_authorization", session.User))
			}
			conn.Write(encodeReadyForQuery('I'))
			return nil
		}

		conn.Write(encodeErrorResponse("FATAL", "08004", "unsupported protocol version"))
		return fmt.Errorf("unsupported protocol version: %d", versionOrCode)
	}
}

// executeQuery compiles and executes SQL string statements, returning wire responses to the client.
func (s *Server) executeQuery(ctx context.Context, conn net.Conn, session *Session, query string) {
	lex := lexer.NewLexer("query", query)
	tokens := lex.Tokenize()
	if lex.Diagnostics().HasErrors() {
		conn.Write(encodeErrorResponse("ERROR", "42601", fmt.Sprintf("lexical error: %v", lex.Diagnostics().AsError())))
		conn.Write(encodeReadyForQuery('I'))
		return
	}

	source := &diagnostic.Source{Name: "query", Text: query}
	p := parser.New(tokens, source)
	prog, err := p.Parse()
	if err != nil || prog == nil || len(prog.Statements) == 0 {
		conn.Write(encodeErrorResponse("ERROR", "42601", fmt.Sprintf("syntax error: %v", err)))
		conn.Write(encodeReadyForQuery('I'))
		return
	}

	for _, statement := range prog.Statements {
		pl := planner.New(s.catalog)
		pSess := planner.Session{ActiveDatabase: session.ActiveDatabase, Ctx: ctx}
		plan, diagList, err := pl.Plan(statement, pSess, source)
		if err != nil || diagList.HasErrors() {
			var errDetail interface{} = err
			if err == nil {
				errDetail = diagList.AsError()
			}
			conn.Write(encodeErrorResponse("ERROR", "42P01", fmt.Sprintf("planning error: %v", errDetail)))
			conn.Write(encodeReadyForQuery('I'))
			return
		}

		result, err := s.exec.Execute(ctx, plan)
		if err != nil {
			conn.Write(encodeErrorResponse("ERROR", "XX000", fmt.Sprintf("execution error: %v", err)))
			conn.Write(encodeReadyForQuery('I'))
			return
		}

		if result.SessionUpdate != nil && result.SessionUpdate.ActiveDatabase != "" {
			session.ActiveDatabase = result.SessionUpdate.ActiveDatabase
		}

		if result.Type == executor.ResultQuery {
			descBuff := newMessageBuffer()
			descBuff.writeInt16(int16(len(result.Columns)))
			for _, col := range result.Columns {
				descBuff.writeCString(col.Name)
				descBuff.writeInt32(0)
				descBuff.writeInt16(0)
				info := MapASTTypeToOID(col.Type)
				descBuff.writeInt32(info.OID)
				descBuff.writeInt16(info.Size)
				descBuff.writeInt32(-1)
				descBuff.writeInt16(0)
			}
			conn.Write(descBuff.finish(MessageRowDescription))

			for _, r := range result.Rows {
				dataBuf := newMessageBuffer()
				dataBuf.writeInt16(int16(len(r)))
				for _, val := range r {
					valBytes, isNull := FormatColumnValue(val)
					if isNull {
						dataBuf.writeInt32(-1)
					} else {
						dataBuf.writeInt32(int32(len(valBytes)))
						dataBuf.writeBytes(valBytes)
					}
				}
				conn.Write(dataBuf.finish(MessageDataRow))
			}
		}

		tag := "SELECT 0"
		switch result.Type {
		case executor.ResultQuery:
			tag = fmt.Sprintf("SELECT %d", len(result.Rows))
		case executor.ResultDML:
			switch statement.(type) {
			case *ast.InsertStmt:
				tag = fmt.Sprintf("INSERT 0 %d", result.RowsAffected)
			case *ast.DeleteStmt:
				tag = fmt.Sprintf("DELETE %d", result.RowsAffected)
			default:
				tag = fmt.Sprintf("UPDATE %d", result.RowsAffected)
			}
		case executor.ResultDDL:
			if result.Message != "" {
				tag = result.Message
			} else {
				tag = "OK"
			}
		}
		conn.Write(encodeCommandComplete(tag))
	}

	conn.Write(encodeReadyForQuery('I'))
}
