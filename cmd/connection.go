package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

const (
	sslRequestCode  = 80877103
	protocolVersion = 196608
)

func HandleSslRequest(conn net.Conn) []byte {
	lengthBuffer := make([]byte, 4)
	io.ReadFull(conn, lengthBuffer)

	protocolBuffer := make([]byte, 4)
	io.ReadFull(conn, protocolBuffer)

	protocol := binary.BigEndian.Uint32(protocolBuffer)

	switch protocol {
	case sslRequestCode:
		conn.Write([]byte("N"))
		newLengthBuffer := make([]byte, 4)
		io.ReadFull(conn, newLengthBuffer)
		io.ReadFull(conn, make([]byte, 4))
		return newLengthBuffer
	case protocolVersion:
		return lengthBuffer
	default:
		return lengthBuffer
	}
}

func HandleStartup(conn net.Conn, length []byte) (*sql.DB, string, error) {
	bodyLength := binary.BigEndian.Uint32(length)
	bodyBuffer := make([]byte, bodyLength-8)
	io.ReadFull(conn, bodyBuffer)
	bodyStr := string(bodyBuffer)

	parts := strings.Split(bodyStr, "\x00")
	parts = parts[:len(parts)-1]

	payload := make(map[string]string)
	for i := 0; i < len(parts)-1; i += 2 {
		payload[parts[i]] = parts[i+1]
	}

	// Send AuthenticationOk
	conn.Write([]byte("R"))
	binary.Write(conn, binary.BigEndian, uint32(8))
	binary.Write(conn, binary.BigEndian, uint32(0))

	// Send ParameterStatus messages
	SendParameterStatus(conn, "standard_conforming_strings", "on")
	SendParameterStatus(conn, "client_encoding", "UTF8")
	SendParameterStatus(conn, "server_version", "16.0")
	SendParameterStatus(conn, "integer_datetimes", "on")

	db, err := HandleTenantDb(payload["database"])
	if err != nil {
		return nil, "", err
	}
	return db, payload["database"], nil
}

func SendParameterStatus(conn net.Conn, name, value string) error {
	var payload bytes.Buffer
	payload.WriteString(name)
	payload.WriteByte(0)
	payload.WriteString(value)
	payload.WriteByte(0)

	var msg bytes.Buffer
	msg.WriteByte('S')
	binary.Write(&msg, binary.BigEndian, uint32(payload.Len()+4))
	msg.Write(payload.Bytes())

	_, err := conn.Write(msg.Bytes())
	return err
}

func handleReadQuery(conn net.Conn, dbName string, statement string) error {
	randomReplicaURL := replicaUrls[0]
	repConn, err := GetOrCreateReplicaConn(dbName, randomReplicaURL)
	if err != nil {
		log.Println(err)
		return err
	}
	err = sendQuery(repConn.Conn, statement)

	if err != nil {
		log.Println(err)
		return err
	}

	for {

		// read one byte for the type
		msgType := make([]byte, 1)
		if _, err := io.ReadFull(repConn.Conn, msgType); err != nil {
			log.Println(err)
			return err
		}

		// Read 4 bytes: message length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(repConn.Conn, lenBuf); err != nil {
			log.Println(err)
			return err
		}

		msgLength := binary.BigEndian.Uint32(lenBuf)

		msgBuf := make([]byte, msgLength-4)
		if _, err := io.ReadFull(repConn.Conn, msgBuf); err != nil {
			log.Println(err)
			return err
		}

		_, err = conn.Write([]byte(msgType))
		if err != nil {
			log.Println(err)
			return err
		}

		_, err = conn.Write([]byte(lenBuf))
		if err != nil {
			log.Println(err)
			return err
		}

		_, err = conn.Write([]byte(msgBuf))
		if err != nil {
			log.Println(err)
			return err
		}
		if msgType[0] == 'C' {
			return nil
		}
	}
}

func handleWriteQuery(conn net.Conn, dbName string, statement string, db *sql.DB) error {
	columns, rows, rowsAffected, err := HandleExecute(db, statement)
	if err != nil {
		HandleError(conn, err)
		return err
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
	for _, replicaUrl := range replicaUrls {
		go func(url string) {
			replicaConn, err := GetOrCreateReplicaConn(dbName, url)
			if err != nil {
				log.Println(err)
				delete(connections[dbName], url)
				return
			}

			err = sendQuery(replicaConn.Conn, statement)
			if err != nil {
				log.Println(err)
				delete(connections[dbName], url)
				return
			}

			for {
				msgType := make([]byte, 1)
				if _, err := io.ReadFull(replicaConn.Conn, msgType); err != nil {
					log.Println(err)
					return
				}

				lenBuf := make([]byte, 4)

				if _, err := io.ReadFull(replicaConn.Conn, lenBuf); err != nil {
					log.Println(err)
					return
				}

				msgLen := binary.BigEndian.Uint32(lenBuf)

				body := make([]byte, msgLen-4)
				if _, err := io.ReadFull(replicaConn.Conn, body); err != nil {
					log.Println(err)
					return
				}

				switch msgType[0] {
				case 'C':
					return
				case 'E':
					log.Println("Replica returned error")
					return
				}

			}

		}(replicaUrl)
	}
	return nil
}

func handleConnection(conn net.Conn) {
	isInTransaction := false

	lengthBuffer := HandleSslRequest(conn)
	db, dbName, err := HandleStartup(conn, lengthBuffer)
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
		command := ExtractCommand(statement)
		switch command {
		case "BEGIN":
			isInTransaction = true
		case "COMMIT", "ROLLBACK":
			isInTransaction = false
		}

		isRead := IsReadQuery(statement) && !isInTransaction && len(replicaUrls) > 0

		if isRead {
			if err := handleReadQuery(conn, dbName, statement); err != nil {
				break
			}
		} else {

			if err := handleWriteQuery(conn, dbName, statement, db); err != nil {
				break
			}
		}

	}

}

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
