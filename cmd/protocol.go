package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
)

func writeUint32(val uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, val)
	return buf
}

func writeUint16(val uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, val)
	return buf
}

func HandleError(w io.Writer, err error) {
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
	buf.Write(writeUint32(uint32(payload.Len() + 4)))
	buf.Write(payload.Bytes())

	w.Write(buf.Bytes())
	if bw, ok := w.(*bufio.Writer); ok {
		bw.Flush()
	}
}

func ReadyForQuery(w io.Writer, isInTransaction bool) {
	w.Write([]byte{MsgReadyForQuery})
	w.Write(writeUint32(5))
	if isInTransaction {
		w.Write([]byte{TxStatusInTransaction})
	} else {
		w.Write([]byte{TxStatusIdle})
	}

	if bw, ok := w.(*bufio.Writer); ok {
		bw.Flush()
	}

}

func SendDataRow(w io.Writer, values []string) {
	w.Write([]byte{MsgDataRow})

	payloadLength := 2
	for _, val := range values {
		payloadLength += 4 + len(val)
	}

	w.Write(writeUint32(uint32(payloadLength + 4)))
	w.Write(writeUint16(uint16(len(values))))

	for _, val := range values {
		w.Write(writeUint32(uint32(len(val))))
		w.Write([]byte(val))
	}
}
func SendCommandComplete(w io.Writer, statement string, rowsAffected int) {
	var cmdBuf bytes.Buffer
	command := ExtractCommand(statement)

	w.Write([]byte{MsgCommandComplete})

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

	w.Write(writeUint32(uint32(cmdBuf.Len() + 4)))

	w.Write(cmdBuf.Bytes())
}

func SendRowDescription(w io.Writer, columnNames []string) {

	w.Write([]byte{MsgRowDescription})

	// Each column field: name + null terminator + tableOID(4) + columnIndex(2) + typeOID(4) + typeSize(2) + typeModifier(4) + formatCode(2)
	const perColumnOverhead = 1 + 4 + 2 + 4 + 2 + 4 + 2 // 19 bytes of fixed fields per column
	var payloadLength int
	for _, columnName := range columnNames {
		payloadLength += len(columnName) + perColumnOverhead
	}

	w.Write(writeUint32(uint32(payloadLength + 2 + 4))) // +2 for column count, +4 for length field
	w.Write(writeUint16(uint16(len(columnNames))))

	for _, columnName := range columnNames {
		w.Write([]byte(columnName))
		w.Write([]byte{0}) // null terminator

		w.Write(writeUint32(UnknownTableOID))
		w.Write(writeUint16(UnknownColumnIndex))
		w.Write(writeUint32(OIDText))
		w.Write(writeUint16(VariableLengthSize))
		w.Write(writeUint32(UnknownTypeModifier))
		w.Write(writeUint16(FormatText))
	}

}
