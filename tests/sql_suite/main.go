package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// PostgreSQL Wire Protocol Constants
const (
	ProtocolVersion3 = 196608

	MsgQuery     byte = 'Q'
	MsgTerminate byte = 'X'

	MsgAuth            byte = 'R'
	MsgParameterStatus byte = 'S'
	MsgReadyForQuery   byte = 'Z'
	MsgRowDescription  byte = 'T'
	MsgDataRow         byte = 'D'
	MsgCommandComplete byte = 'C'
	MsgErrorResponse   byte = 'E'
)

// ColumnHeader metadata for query result column headers.
type ColumnHeader struct {
	Name    string
	TypeOID int32
	Size    int16
}

// QueryResult encapsulates response data returned from PenguinDB SQL Gateway.
type QueryResult struct {
	Columns        []ColumnHeader
	Rows           [][]string
	CommandTag     string
	ActiveDatabase string
	Error          error
}

// PGClient manages TCP connection and wire protocol communication with PenguinDB SQL Gateway.
type PGClient struct {
	conn     net.Conn
	host     string
	port     int
	user     string
	activeDb string
}

func NewPGClient(host string, port int, user, database string) *PGClient {
	if database == "" {
		database = "postgres"
	}
	if user == "" {
		user = "postgres"
	}
	return &PGClient{
		host:     host,
		port:     port,
		user:     user,
		activeDb: database,
	}
}

func (c *PGClient) Connect() error {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connection failed to %s: %w", addr, err)
	}
	c.conn = conn

	_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send SSLRequest (decline SSL fallback)
	sslReq := []byte{0, 0, 0, 8, 0x04, 0xD2, 0x16, 0x2F}
	if _, err := c.conn.Write(sslReq); err != nil {
		c.Close()
		return fmt.Errorf("SSLRequest write failed: %w", err)
	}

	var sslResp [1]byte
	if _, err := io.ReadFull(c.conn, sslResp[:]); err != nil {
		c.Close()
		return fmt.Errorf("SSL response read failed: %w", err)
	}

	// Send StartupMessage
	var payload bytes.Buffer
	var ver [4]byte
	binary.BigEndian.PutUint32(ver[:], ProtocolVersion3)
	payload.Write(ver[:])
	payload.WriteString(fmt.Sprintf("user\x00%s\x00database\x00%s\x00\x00", c.user, c.activeDb))

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(payload.Len()+4))
	c.conn.Write(lenBuf[:])
	c.conn.Write(payload.Bytes())

	// Read Authentication & ReadyForQuery packet stream
	for {
		var typeByte [1]byte
		if _, err := io.ReadFull(c.conn, typeByte[:]); err != nil {
			c.Close()
			return fmt.Errorf("handshake stream read error: %w", err)
		}
		pLen, err := c.readInt32()
		if err != nil {
			c.Close()
			return err
		}
		pData := make([]byte, pLen-4)
		if _, err := io.ReadFull(c.conn, pData); err != nil {
			c.Close()
			return err
		}

		switch typeByte[0] {
		case MsgAuth:
			// Trust Auth Success
		case MsgParameterStatus:
			// Status parameters
		case MsgErrorResponse:
			c.Close()
			return fmt.Errorf("handshake rejected: %s", string(pData))
		case MsgReadyForQuery:
			_ = c.conn.SetDeadline(time.Time{})
			return nil
		}
	}
}

func (c *PGClient) Close() {
	if c.conn != nil {
		termMsg := []byte{MsgTerminate, 0, 0, 0, 4}
		_, _ = c.conn.Write(termMsg)
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *PGClient) ExecuteQuery(sql string) (*QueryResult, error) {
	if c.conn == nil {
		if err := c.Connect(); err != nil {
			return nil, err
		}
	}

	_ = c.conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() {
		if c.conn != nil {
			_ = c.conn.SetDeadline(time.Time{})
		}
	}()

	var qBuf bytes.Buffer
	qBuf.WriteByte(MsgQuery)
	length := int32(len(sql) + 5)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(length))
	qBuf.Write(lenBuf[:])
	qBuf.WriteString(sql)
	qBuf.WriteByte(0)

	if _, err := c.conn.Write(qBuf.Bytes()); err != nil {
		c.Close()
		return nil, fmt.Errorf("network write error: %w", err)
	}

	res := &QueryResult{
		ActiveDatabase: c.activeDb,
	}

	for {
		var typeByte [1]byte
		if _, err := io.ReadFull(c.conn, typeByte[:]); err != nil {
			c.Close()
			res.Error = fmt.Errorf("network connection lost: %w", err)
			return res, res.Error
		}
		pLen, err := c.readInt32()
		if err != nil {
			c.Close()
			res.Error = err
			return res, res.Error
		}
		if pLen < 4 {
			c.Close()
			res.Error = fmt.Errorf("invalid packet length: %d", pLen)
			return res, res.Error
		}
		pData := make([]byte, pLen-4)
		if _, err := io.ReadFull(c.conn, pData); err != nil {
			c.Close()
			res.Error = err
			return res, res.Error
		}

		switch typeByte[0] {
		case MsgRowDescription:
			r := bytes.NewReader(pData)
			var fieldCount int16
			binary.Read(r, binary.BigEndian, &fieldCount)
			res.Columns = make([]ColumnHeader, fieldCount)
			for i := 0; i < int(fieldCount); i++ {
				name, _ := c.readCString(r)
				var tableOID int32
				var attrNum int16
				var typeOID int32
				var typeSize int16
				var typeMod int32
				var formatCode int16
				binary.Read(r, binary.BigEndian, &tableOID)
				binary.Read(r, binary.BigEndian, &attrNum)
				binary.Read(r, binary.BigEndian, &typeOID)
				binary.Read(r, binary.BigEndian, &typeSize)
				binary.Read(r, binary.BigEndian, &typeMod)
				binary.Read(r, binary.BigEndian, &formatCode)
				res.Columns[i] = ColumnHeader{
					Name:    name,
					TypeOID: typeOID,
					Size:    typeSize,
				}
			}

		case MsgDataRow:
			r := bytes.NewReader(pData)
			var colCount int16
			binary.Read(r, binary.BigEndian, &colCount)
			row := make([]string, colCount)
			for i := 0; i < int(colCount); i++ {
				var valLen int32
				binary.Read(r, binary.BigEndian, &valLen)
				if valLen == -1 {
					row[i] = ""
				} else {
					valBytes := make([]byte, valLen)
					r.Read(valBytes)
					row[i] = string(valBytes)
				}
			}
			res.Rows = append(res.Rows, row)

		case MsgCommandComplete:
			res.CommandTag = string(bytes.TrimRight(pData, "\x00"))
			if strings.HasPrefix(strings.ToUpper(sql), "USE ") {
				parts := strings.Fields(sql)
				if len(parts) >= 2 {
					db := strings.TrimRight(parts[1], ";")
					c.activeDb = db
					res.ActiveDatabase = db
				}
			}

		case MsgErrorResponse:
			res.Error = fmt.Errorf("%s", parseErrorResponse(pData))

		case MsgReadyForQuery:
			return res, res.Error
		}
	}
}

func (c *PGClient) readInt32() (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(c.conn, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf[:])), nil
}

func (c *PGClient) readCString(r io.Reader) (string, error) {
	var buf bytes.Buffer
	var oneByte [1]byte
	for {
		if _, err := io.ReadFull(r, oneByte[:]); err != nil {
			return "", err
		}
		if oneByte[0] == 0 {
			break
		}
		buf.WriteByte(oneByte[0])
	}
	return buf.String(), nil
}

func parseErrorResponse(pData []byte) string {
	var parts []string
	r := bytes.NewReader(pData)
	for r.Len() > 0 {
		typeByte, err := r.ReadByte()
		if err != nil || typeByte == 0 {
			break
		}
		var buf bytes.Buffer
		for {
			b, err := r.ReadByte()
			if err != nil || b == 0 {
				break
			}
			buf.WriteByte(b)
		}
		str := buf.String()
		switch typeByte {
		case 'M':
			parts = append(parts, str)
		case 'C':
			parts = append(parts, fmt.Sprintf("(code %s)", str))
		case 'S':
			parts = append(parts, fmt.Sprintf("[%s]", str))
		}
	}
	if len(parts) == 0 {
		return strings.ReplaceAll(string(pData), "\x00", " ")
	}
	return strings.Join(parts, " ")
}

// SplitSQLStatements splits raw SQL file content by semicolons while respecting blank-line separated statements.
func SplitSQLStatements(content string) []string {
	var stmts []string
	rawStmts := strings.Split(content, ";")
	for _, raw := range rawStmts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		// Strip comments if any
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

func main() {
	fileFlag := flag.String("file", "", "Path to input .sql file")
	outFlag := flag.String("out", "", "Path to output .pengout file")
	hostFlag := flag.String("host", "127.0.0.1", "PenguinDB Gateway host")
	portFlag := flag.Int("port", 5433, "PenguinDB Gateway port")
	userFlag := flag.String("user", "postgres", "Database user")
	dbFlag := flag.String("database", "postgres", "Database name")

	flag.Parse()

	if *fileFlag == "" || *outFlag == "" {
		fmt.Println("Usage: main -file <path.sql> -out <path.pengout> [-host <ip>] [-port <port>]")
		os.Exit(1)
	}

	contentBytes, err := os.ReadFile(*fileFlag)
	if err != nil {
		fmt.Printf("Error reading SQL file %s: %v\n", *fileFlag, err)
		os.Exit(1)
	}

	stmts := SplitSQLStatements(string(contentBytes))

	client := NewPGClient(*hostFlag, *portFlag, *userFlag, *dbFlag)
	if err := client.Connect(); err != nil {
		fmt.Printf("Error connecting to PenguinDB on %s:%d: %v\n", *hostFlag, *portFlag, err)
		os.Exit(1)
	}
	defer client.Close()

	outFile, err := os.Create(*outFlag)
	if err != nil {
		fmt.Printf("Error creating output file %s: %v\n", *outFlag, err)
		os.Exit(1)
	}
	defer outFile.Close()

	csvWriter := csv.NewWriter(outFile)

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

		res, err := client.ExecuteQuery(query)
		if err != nil || (res != nil && res.Error != nil) {
			errStr := err
			if res != nil && res.Error != nil {
				errStr = res.Error
			}
			fmt.Fprintf(outFile, "ERROR: %v\n", errStr)
			continue
		}

		if len(res.Columns) > 0 {
			headers := make([]string, len(res.Columns))
			for i, col := range res.Columns {
				headers[i] = col.Name
			}
			_ = csvWriter.Write(headers)
			for _, row := range res.Rows {
				_ = csvWriter.Write(row)
			}
			csvWriter.Flush()
		}
	}
}
