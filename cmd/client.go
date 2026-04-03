package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strings"
	"sync/atomic"
)

var counter uint64

func HandleSslRequest(conn net.Conn) []byte {
	lengthBuffer := make([]byte, 4)
	io.ReadFull(conn, lengthBuffer)

	protocolBuffer := make([]byte, 4)
	io.ReadFull(conn, protocolBuffer)

	protocol := binary.BigEndian.Uint32(protocolBuffer)

	switch protocol {
	case SSLRequestCode:
		conn.Write([]byte("N")) // 'N' = SSL not supported
		newLengthBuffer := make([]byte, 4)
		io.ReadFull(conn, newLengthBuffer)
		io.ReadFull(conn, make([]byte, 4))
		return newLengthBuffer
	case GSSEncRequestCode:
		conn.Write([]byte("N")) // 'N' = GSS encryption not supported
		return HandleSslRequest(conn)
	case ProtocolVersion3:
		return lengthBuffer
	default:
		return lengthBuffer
	}
}

func HandleStartup(conn net.Conn, w io.Writer, length []byte) (*sql.DB, string, error) {
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

	w.Write([]byte{MsgAuthentication})
	binary.Write(w, binary.BigEndian, uint32(8))
	binary.Write(w, binary.BigEndian, uint32(0)) // AuthenticationOk

	// Send ParameterStatus messages
	SendParameterStatus(w, "standard_conforming_strings", "on")
	SendParameterStatus(w, "client_encoding", "UTF8")
	SendParameterStatus(w, "server_version", "16.0")
	SendParameterStatus(w, "integer_datetimes", "on")

	db, err := GetOrCreateDb(payload["database"])
	if err != nil {
		return nil, "", err
	}
	return db, payload["database"], nil
}

func SendParameterStatus(w io.Writer, name, value string) error {
	var payload bytes.Buffer
	payload.WriteString(name)
	payload.WriteByte(0)
	payload.WriteString(value)
	payload.WriteByte(0)

	var msg bytes.Buffer
	msg.WriteByte(MsgParameterStatus)
	binary.Write(&msg, binary.BigEndian, uint32(payload.Len()+4))
	msg.Write(payload.Bytes())

	_, err := w.Write(msg.Bytes())
	return err
}

func handleConnection(conn net.Conn, w io.Writer) {
	isInTransaction := false

	lengthBuffer := HandleSslRequest(conn)
	db, dbName, err := HandleStartup(conn, w, lengthBuffer)
	if err != nil {
		conn.Close()
		return
	}

	for {
		ReadyForQuery(w, isInTransaction)
		statement, err := HandleStatement(conn, w)
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
			if err := handleReadQuery(w, dbName, statement); err != nil {
				break
			}
		} else {

			if err := handleWriteQuery(w, dbName, statement, db); err != nil {
				break
			}
		}

	}

}

func handleReadQuery(w io.Writer, dbName string, statement string) error {
	index := atomic.AddUint64(&counter, 1) % uint64(len(replicaUrls))

	randomReplicaURL := replicaUrls[int(index)]

	repConn, err := GetOrCreateReplicaConn(dbName, randomReplicaURL)
	if err != nil {
		log.Println(err)
		delete(connections[dbName], randomReplicaURL)
		return nil
	}
	err = sendQuery(repConn.Conn, statement)

	if err != nil {
		log.Println(err)
		delete(connections[dbName], randomReplicaURL)
		HandleError(w, err)
		return nil
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

		_, err = w.Write([]byte(msgType))
		if err != nil {
			log.Println(err)
			return err
		}

		_, err = w.Write([]byte(lenBuf))
		if err != nil {
			log.Println(err)
			return err
		}

		_, err = w.Write([]byte(msgBuf))
		if err != nil {
			log.Println(err)
			return err
		}
		if msgType[0] == MsgCommandComplete {
			return nil
		}
	}
}

func handleWriteQuery(w io.Writer, dbName string, statement string, db *sql.DB) error {
	rowsAffected, err := HandleExecute(db, w, statement)
	if err != nil {
		HandleError(w, err)
		return nil
	}

	SendCommandComplete(w, statement, rowsAffected)

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
				case MsgCommandComplete:
					return
				case MsgErrorResponse:
					log.Println("Replica returned error")
					return
				}

			}

		}(replicaUrl)
	}
	return nil
}
