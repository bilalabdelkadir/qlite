package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

func HandleSslRequest(conn net.Conn) []byte {
	lengthBuffer := make([]byte, 4)
	io.ReadFull(conn, lengthBuffer)

	protocolBuffer := make([]byte, 4)
	io.ReadFull(conn, protocolBuffer)

	protocol := binary.BigEndian.Uint32(protocolBuffer)

	switch protocol {
	case 80877103:
		conn.Write([]byte("N"))
		newLengthBuffer := make([]byte, 4)
		io.ReadFull(conn, newLengthBuffer)
		io.ReadFull(conn, make([]byte, 4))
		return newLengthBuffer
	case 196608:
		return lengthBuffer
	default:
		return lengthBuffer
	}
}

func HandleStartup(conn net.Conn, length []byte) (*sql.DB, error) {

	bodyLength := binary.BigEndian.Uint32(length)

	bodyBuffer := make([]byte, bodyLength-8)

	io.ReadFull(conn, bodyBuffer)

	bodyStr := string(bodyBuffer)
	fmt.Println("this is the body", bodyStr)

	parts := strings.Split(bodyStr, "\x00")
	parts = parts[:len(parts)-1]

	payload := make(map[string]string)

	for i := 0; i < len(parts)-1; i = i + 2 {
		payload[parts[i]] = parts[i+1]

	}

	for k, v := range payload {
		fmt.Printf("the key %s and the value %s\n", k, v)
	}

	conn.Write([]byte("R"))
	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, 8)

	conn.Write(lengthBuffer)

	authBuffer := make([]byte, 4)

	binary.BigEndian.PutUint32(authBuffer, 0)

	conn.Write(authBuffer)

	db, err := HandleTenantDb(payload["database"])
	if err != nil {
		return nil, err

	}
	return db, nil

}

func handleConnection(conn net.Conn) {

	lengthBuffer := HandleSslRequest(conn)
	db, err := HandleStartup(conn, lengthBuffer)
	if err != nil {
		conn.Close()
		return
	}
	defer db.Close()

	for {
		ReadyForQuery(conn)
		statement, err := HandleStatement(conn)
		if err != nil {
			break
		}
		columns, rows, rowsAffected, err := HandleExecute(db, statement)
		if err != nil {
			HandleError(conn, err)
			continue
		}
		if columns != nil {
			SendRowDescription(conn, columns)

			for _, row := range rows {
				SendDataRow(conn, row)
			}
			SendCommandComplete(conn, statement, rowsAffected)

		} else {
			SendCommandComplete(conn, statement, rowsAffected)

		}

	}

}
