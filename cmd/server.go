package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// PostgreSQL wire protocol message types (frontend -> backend).
const (
	// MsgQuery is the Simple Query message ('Q').
	MsgQuery byte = 'Q'
	// MsgTerminate is sent by the client to close the connection ('X').
	MsgTerminate byte = 'X'
)

// PostgreSQL wire protocol message types (backend -> frontend).
const (
	// MsgAuthentication is the authentication response ('R').
	MsgAuthentication byte = 'R'
	// MsgParameterStatus sends a runtime parameter value ('S').
	MsgParameterStatus byte = 'S'
	// MsgRowDescription describes the columns in a query result ('T').
	MsgRowDescription byte = 'T'
	// MsgDataRow carries a single row of query results ('D').
	MsgDataRow byte = 'D'
	// MsgCommandComplete signals a command finished successfully ('C').
	MsgCommandComplete byte = 'C'
	// MsgErrorResponse reports an error to the client ('E').
	MsgErrorResponse byte = 'E'
	// MsgReadyForQuery tells the client the backend is ready ('Z').
	MsgReadyForQuery byte = 'Z'
)

// Error response field identifiers.
const (
	// FieldSeverity identifies the severity field in an error message.
	FieldSeverity byte = 'S'
	// FieldMessage identifies the human-readable message field.
	FieldMessage byte = 'M'
)

// Transaction status indicators sent in ReadyForQuery.
const (
	// TxStatusIdle means no transaction is active.
	TxStatusIdle byte = 'I'
	// TxStatusInTransaction means a transaction is in progress.
	TxStatusInTransaction byte = 'T'
)

// PostgreSQL type OIDs and format codes.
const (
	// OIDText is the PostgreSQL OID for the TEXT type.
	OIDText uint32 = 25
	// UnknownTableOID means the column is not tied to a specific table.
	UnknownTableOID uint32 = 0
	// UnknownColumnIndex means the column index is not available.
	UnknownColumnIndex uint16 = 0
	// UnknownTypeModifier means no type-specific modifier is applied (-1 as uint32).
	UnknownTypeModifier uint32 = 0xFFFFFFFF
	// VariableLengthSize indicates the type has no fixed size (-1 as int16 / 0xFFFF as uint16).
	VariableLengthSize uint16 = 0xFFFF
	// FormatText indicates text format for result columns.
	FormatText uint16 = 0
)

// Startup and SSL negotiation codes.
const (
	// SSLRequestCode is the magic number clients send to request SSL.
	SSLRequestCode uint32 = 80877103
	// ProtocolVersion3 is the PostgreSQL 3.0 protocol version number.
	ProtocolVersion3 uint32 = 196608
)

func sendQuery(conn net.Conn, statement string) error {
	var statementToSend bytes.Buffer

	statementToSend.WriteString(statement)
	statementToSend.WriteByte(0)

	_, err := conn.Write([]byte{MsgQuery})
	if err != nil {
		log.Println(err)
		return err
	}

	err = binary.Write(conn, binary.BigEndian, uint32(4+statementToSend.Len()))

	if err != nil {
		log.Println(err)
		return err
	}

	_, err = conn.Write(statementToSend.Bytes())
	if err != nil {
		log.Println(err)
		return err
	}

	return nil

}

func ReadStartupResponse(conn net.Conn, dbName string) error {
	// Read 1 byte: message type
	msgType := make([]byte, 1)
	if _, err := io.ReadFull(conn, msgType); err != nil {
		return err
	}

	if msgType[0] == MsgErrorResponse {
		return fmt.Errorf("replica rejected connection for database %s", dbName)
	}

	// Read 4 bytes: message length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return err
	}

	// Read 4 bytes: authentication type
	authBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, authBuf); err != nil {
		return err
	}

	// Read ReadyForQuery: 1 byte message type ('Z')
	zType := make([]byte, 1)
	if _, err := io.ReadFull(conn, zType); err != nil {
		return err
	}

	// Read 4 bytes: length of ReadyForQuery message
	rfqLen := make([]byte, 4)
	if _, err := io.ReadFull(conn, rfqLen); err != nil {
		return err
	}

	// Read 1 byte: idle status ('I')
	idleStatus := make([]byte, 1)
	if _, err := io.ReadFull(conn, idleStatus); err != nil {
		return err
	}

	return nil
}

func GetOrCreateDb(dbName string) (*sql.DB, error) {
	clientsMu.RLock()
	db := clients[dbName]
	log.Printf("GetOrCreateDb: %s (exists: %v)", dbName, clients[dbName] != nil)

	clientsMu.RUnlock()

	if db != nil {
		return db, nil
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()

	db = clients[dbName]
	if db != nil {
		return db, nil
	}

	newDB, err := HandleTenantDb(dbName)
	if err != nil {
		return nil, err
	}

	clients[dbName] = newDB
	return newDB, nil
}

func GetOrCreateReplicaConn(dbName string, replicaURL string) (*replicaConn, error) {
	connectionsMu.Lock()
	defer connectionsMu.Unlock()
	if connections[dbName] == nil {
		connections[dbName] = make(map[string]*replicaConn)
	}

	if connections[dbName][replicaURL] == nil {
		conn, err := net.DialTimeout("tcp", replicaURL, 5*time.Second)
		if err != nil {
			return nil, err
		}

		err = SendStartupMessage(conn, dbName)
		if err != nil {
			return nil, err
		}

		err = ReadStartupResponse(conn, dbName)
		if err != nil {
			return nil, err
		}

		connections[dbName][replicaURL] = &replicaConn{
			Conn:     conn,
			LastUsed: time.Now(),
		}
	}

	return connections[dbName][replicaURL], nil
}
func SendStartupMessage(conn net.Conn, dbName string) error {
	var body bytes.Buffer

	body.WriteString("user")
	body.WriteByte(0)
	body.WriteString("postgres")
	body.WriteByte(0)
	body.WriteString("database")
	body.WriteByte(0)
	body.WriteString(dbName)
	body.WriteByte(0)
	body.WriteByte(0)

	totalLen := 4 + 4 + body.Len()

	err := binary.Write(conn, binary.BigEndian, uint32(totalLen))
	if err != nil {
		return err
	}
	err = binary.Write(conn, binary.BigEndian, ProtocolVersion3)
	if err != nil {
		return err
	}
	_, err = conn.Write(body.Bytes())

	return err

}
