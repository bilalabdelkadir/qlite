package main

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	listener, err := net.Listen("tcp", ":5433")
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "./test.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("tcp connection is running on 5433 happy hacking")
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			HandleError(conn, err)
		}

		HandleSslRequest(conn)
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

}

func ExtractCommand(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}

	parts := strings.Fields(statement)
	command := strings.ToUpper(parts[0])

	return command

}

func HandleError(conn net.Conn, err error) {
	conn.Write([]byte("E"))
	severity := "ERROR\x00"
	message := err.Error() + "\x00"

	endMarker := []byte{0}

	payloadLength := len(severity) + 1 + len(message) + 1 + len(endMarker)
	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, uint32(payloadLength+4))

	conn.Write(lengthBuffer)

	conn.Write([]byte("S"))
	conn.Write([]byte(severity))
	conn.Write([]byte("M"))
	conn.Write([]byte(message))
	conn.Write(endMarker)

}

func HandleExecute(db *sql.DB, statement string) (columns []string, rows [][]string, rowsAffected int, err error) {
	command := ExtractCommand(statement)

	switch command {
	case "SELECT":
		r, e := db.Query(statement)
		if e != nil {
			return nil, nil, 0, e
		}
		defer r.Close()

		cols, _ := r.Columns()
		var results [][]string

		for r.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			r.Scan(valuePtrs...)
			row := make([]string, len(cols))
			for i, val := range values {
				if val != nil {
					row[i] = fmt.Sprintf("%v", val)
				} else {
					row[i] = "NULL"
				}
			}
			results = append(results, row)
		}

		return cols, results, len(results), nil

	case "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER":
		res, e := db.Exec(statement)
		if e != nil {
			return nil, nil, 0, e
		}
		ra, _ := res.RowsAffected()
		return nil, nil, int(ra), nil

	default:
		return nil, nil, 0, fmt.Errorf("unsupported SQL command: %s", command)
	}
}
func HandleSslRequest(conn net.Conn) {
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
		HandleStartup(conn, newLengthBuffer)
	case 196608:
		HandleStartup(conn, lengthBuffer)
	default:
		HandleStartup(conn, lengthBuffer)
	}
}

func HandleStartup(conn net.Conn, length []byte) {

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

}

func HandleStatement(conn net.Conn) (string, error) {
	typeBuffer := make([]byte, 1)

	io.ReadFull(conn, typeBuffer)

	msgType := string(typeBuffer)

	if msgType != "Q" {
		HandleError(conn, errors.New("Wrong type."))
	}

	statementLengthBuffer := make([]byte, 4)

	io.ReadFull(conn, statementLengthBuffer)

	statementLength := binary.BigEndian.Uint32(statementLengthBuffer)

	statementBuffer := make([]byte, statementLength-4)

	_, err := io.ReadFull(conn, statementBuffer)
	if err != nil {
		return "", err
	}

	statement := string(statementBuffer)

	return statement, nil
}

func ReadyForQuery(conn net.Conn) {

	conn.Write([]byte("Z"))
	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, 5)
	conn.Write(lengthBuffer)

	conn.Write([]byte("I"))

}

func SendDataRow(conn net.Conn, values []string) {
	conn.Write([]byte("D"))

	payloadLength := 2
	for _, val := range values {
		payloadLength += 4 + len(val)
	}

	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, uint32(payloadLength+4))
	conn.Write(lengthBuffer)

	columnCount := make([]byte, 2)
	binary.BigEndian.PutUint16(columnCount, uint16(len(values)))
	conn.Write(columnCount)

	for _, val := range values {
		buf4 := make([]byte, 4)
		binary.BigEndian.PutUint32(buf4, uint32(len(val)))
		conn.Write(buf4)

		conn.Write([]byte(val))
	}
}

func SendCommandComplete(conn net.Conn, statement string, rowsAffected int) {
	command := ExtractCommand(statement)

	conn.Write([]byte("C"))

	var cmdToSend string

	switch command {
	case "INSERT":
		cmdToSend = fmt.Sprintf("INSERT 0 %d\x00", rowsAffected)
	case "UPDATE":
		cmdToSend = fmt.Sprintf("UPDATE %d\x00", rowsAffected)
	case "DELETE":
		cmdToSend = fmt.Sprintf("DELETE %d\x00", rowsAffected)
	case "SELECT":
		cmdToSend = fmt.Sprintf("SELECT %d\x00", rowsAffected)
	case "CREATE", "DROP", "ALTER":
		cmdToSend = fmt.Sprintf("%s\x00", command)
	default:
		cmdToSend = fmt.Sprintf("%s\x00", command)
	}

	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, uint32(len(cmdToSend)+4)) // length includes itself
	conn.Write(lengthBuffer)

	conn.Write([]byte(cmdToSend))
}

func SendRowDescription(conn net.Conn, columnNames []string) {

	conn.Write([]byte("T"))

	var payloadLength int

	for _, columnName := range columnNames {
		payloadLength += len(columnName) + 1 + 4 + 2 + 4 + 2 + 4 + 2
	}

	lengthBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuffer, uint32(payloadLength+2+4))
	conn.Write(lengthBuffer)

	columnCount := make([]byte, 2)
	binary.BigEndian.PutUint16(columnCount, uint16(len(columnNames)))
	conn.Write(columnCount)

	for _, columnName := range columnNames {
		conn.Write([]byte(columnName + "\x00"))

		buf4 := make([]byte, 4)
		binary.BigEndian.PutUint32(buf4, 0)
		conn.Write(buf4)

		buf2 := make([]byte, 2)
		binary.BigEndian.PutUint16(buf2, 0)
		conn.Write(buf2)

		binary.BigEndian.PutUint32(buf4, 25)
		conn.Write(buf4)

		binary.BigEndian.PutUint16(buf2, uint16(0xFFFF))
		conn.Write(buf2)

		binary.BigEndian.PutUint32(buf4, 0xFFFFFFFF)
		conn.Write(buf4)

		binary.BigEndian.PutUint16(buf2, 0)
		conn.Write(buf2)
	}

}
