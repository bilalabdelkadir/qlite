package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const (
	sslRequestCode  = 80877103
	protocolVersion = 196608
)

func sendQuery(conn net.Conn, statement string) error {
	var statementToSend bytes.Buffer

	statementToSend.WriteString(statement)
	statementToSend.WriteByte(0)

	_, err := conn.Write([]byte("Q"))
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

	if msgType[0] == 'E' {
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
	err = binary.Write(conn, binary.BigEndian, uint32(protocolVersion))
	if err != nil {
		return err
	}
	_, err = conn.Write(body.Bytes())

	return err

}
