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

	for i := 0; i < len(parts)-1; i = i + 2 {
		payload[parts[i]] = parts[i+1]

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
		return nil, "", err

	}
	return db, payload["database"], nil

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

			randomReplicaURL := replicaUrls[0]
			repConn, err := GetOrCreateReplicaConn(dbName, randomReplicaURL)
			if err != nil {
				log.Println(err)
				break
			}
			err = sendQuery(repConn.Conn, statement)

			if err != nil {
				log.Println(err)
				break
			}

			for {

				// read one byte for the type
				msgType := make([]byte, 1)
				if _, err := io.ReadFull(repConn.Conn, msgType); err != nil {
					log.Println(err)
					break
				}

				// Read 4 bytes: message length
				lenBuf := make([]byte, 4)
				if _, err := io.ReadFull(repConn.Conn, lenBuf); err != nil {
					log.Println(err)
					break
				}

				msgLength := binary.BigEndian.Uint32(lenBuf)

				msgBuf := make([]byte, msgLength-4)
				if _, err := io.ReadFull(repConn.Conn, msgBuf); err != nil {
					log.Println(err)
					break
				}

				_, err = conn.Write([]byte(msgType))
				if err != nil {
					log.Println(err)
					break
				}

				_, err = conn.Write([]byte(lenBuf))
				if err != nil {
					log.Println(err)
					break
				}

				_, err = conn.Write([]byte(msgBuf))
				if err != nil {
					log.Println(err)
					break
				}
				if msgType[0] == 'C' {

					break
				}
			}

		} else {
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

	totalLen := uint32(4 + statementToSend.Len())

	lenBuf := make([]byte, 4)

	binary.BigEndian.PutUint32(lenBuf, uint32(totalLen))

	_, err = conn.Write(lenBuf)
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
