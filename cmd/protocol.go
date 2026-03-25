package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
)

func HandleError(conn net.Conn, err error) {
	var buf bytes.Buffer
	var payload bytes.Buffer

	severity := "ERROR"
	message := err.Error()

	// Write fields
	payload.WriteByte('S')
	payload.WriteString(severity)
	payload.WriteByte(0)

	payload.WriteByte('M')
	payload.WriteString(message)
	payload.WriteByte(0)

	// End marker
	payload.WriteByte(0)

	// Write message type
	buf.WriteByte('E')

	// Write length (payload + length field itself)
	length := uint32(payload.Len() + 4)
	binary.Write(&buf, binary.BigEndian, length)

	// Write payload
	buf.Write(payload.Bytes())

	// Send everything
	conn.Write(buf.Bytes())
}

func ReadyForQuery(conn net.Conn) {
	conn.Write([]byte("Z"))
	binary.Write(conn, binary.BigEndian, uint32(5))
	conn.Write([]byte("I"))
}

func SendDataRow(conn net.Conn, values []string) {
	conn.Write([]byte("D"))

	payloadLength := 2
	for _, val := range values {
		payloadLength += 4 + len(val)
	}

	binary.Write(conn, binary.BigEndian, uint32(payloadLength+4))
	binary.Write(conn, binary.BigEndian, uint16(len(values)))

	for _, val := range values {
		binary.Write(conn, binary.BigEndian, uint32(len(val)))
		conn.Write([]byte(val))
	}
}
func SendCommandComplete(conn net.Conn, statement string, rowsAffected int) {
	var cmdBuf bytes.Buffer
	command := ExtractCommand(statement)

	conn.Write([]byte("C"))

	switch command {
	case "INSERT":
		cmdBuf.WriteString("INSERT 0 ")
		cmdBuf.WriteString(strconv.Itoa(rowsAffected))
	case "UPDATE":
		cmdBuf.WriteString("UPDATE ")
		cmdBuf.WriteString(strconv.Itoa(rowsAffected))
	case "DELETE":
		cmdBuf.WriteString("DELETE ")
		cmdBuf.WriteString(strconv.Itoa(rowsAffected))
	case "SELECT":
		cmdBuf.WriteString("SELECT ")
		cmdBuf.WriteString(strconv.Itoa(rowsAffected))
	default:
		cmdBuf.WriteString(command)
	}

	cmdBuf.WriteByte(0)

	binary.Write(conn, binary.BigEndian, uint32(cmdBuf.Len()+4))

	conn.Write(cmdBuf.Bytes())
}

func SendRowDescription(conn net.Conn, columnNames []string) {

	conn.Write([]byte("T"))

	var payloadLength int

	for _, columnName := range columnNames {
		payloadLength += len(columnName) + 1 + 4 + 2 + 4 + 2 + 4 + 2
	}

	binary.Write(conn, binary.BigEndian, uint32(payloadLength+2+4))
	binary.Write(conn, binary.BigEndian, uint16(len(columnNames)))

	for _, columnName := range columnNames {
		conn.Write([]byte(columnName))
		conn.Write([]byte{0})

		binary.Write(conn, binary.BigEndian, uint32(0))
		binary.Write(conn, binary.BigEndian, uint16(0))
		binary.Write(conn, binary.BigEndian, uint32(25))
		binary.Write(conn, binary.BigEndian, uint16(0xFFFF))
		binary.Write(conn, binary.BigEndian, uint32(0xFFFFFFFF))
		binary.Write(conn, binary.BigEndian, uint16(0))
	}

}
