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

	payload.WriteByte(FieldSeverity)
	payload.WriteString("ERROR")
	payload.WriteByte(0)

	payload.WriteByte(FieldMessage)
	payload.WriteString(err.Error())
	payload.WriteByte(0)

	payload.WriteByte(0) // end marker

	buf.WriteByte(MsgErrorResponse)
	binary.Write(&buf, binary.BigEndian, uint32(payload.Len()+4))
	buf.Write(payload.Bytes())

	conn.Write(buf.Bytes())
}

func ReadyForQuery(conn net.Conn, isInTransaction bool) {
	conn.Write([]byte{MsgReadyForQuery})
	binary.Write(conn, binary.BigEndian, uint32(5))
	if isInTransaction {
		conn.Write([]byte{TxStatusInTransaction})
	} else {
		conn.Write([]byte{TxStatusIdle})
	}
}

func SendDataRow(conn net.Conn, values []string) {
	conn.Write([]byte{MsgDataRow})

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

	conn.Write([]byte{MsgCommandComplete})

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

	conn.Write([]byte{MsgRowDescription})

	// Each column field: name + null terminator + tableOID(4) + columnIndex(2) + typeOID(4) + typeSize(2) + typeModifier(4) + formatCode(2)
	const perColumnOverhead = 1 + 4 + 2 + 4 + 2 + 4 + 2 // 19 bytes of fixed fields per column
	var payloadLength int
	for _, columnName := range columnNames {
		payloadLength += len(columnName) + perColumnOverhead
	}

	binary.Write(conn, binary.BigEndian, uint32(payloadLength+2+4)) // +2 for column count, +4 for length field
	binary.Write(conn, binary.BigEndian, uint16(len(columnNames)))

	for _, columnName := range columnNames {
		conn.Write([]byte(columnName))
		conn.Write([]byte{0}) // null terminator

		binary.Write(conn, binary.BigEndian, UnknownTableOID)
		binary.Write(conn, binary.BigEndian, UnknownColumnIndex)
		binary.Write(conn, binary.BigEndian, OIDText)
		binary.Write(conn, binary.BigEndian, VariableLengthSize)
		binary.Write(conn, binary.BigEndian, UnknownTypeModifier)
		binary.Write(conn, binary.BigEndian, FormatText)
	}

}
