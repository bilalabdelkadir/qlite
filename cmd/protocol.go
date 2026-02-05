package main

import (
	"encoding/binary"
	"fmt"
	"net"
)

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
