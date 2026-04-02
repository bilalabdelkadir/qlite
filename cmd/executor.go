package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func IsReadQuery(statement string) bool {
	s := strings.TrimSpace(strings.ToUpper(statement))

	switch {
	case strings.HasPrefix(s, "SELECT"):
		return true
	case strings.HasPrefix(s, "PRAGMA"):
		return true
	case strings.HasPrefix(s, "EXPLAIN"):
		return true
	default:
		return false
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

func HandleExecute(db *sql.DB, w io.Writer, statement string) (int, error) {
	command := ExtractCommand(statement)

	rowCount := 0
	switch command {
	case "SET":
		return 0, nil
	case "BRANCH":
		parts := strings.Fields(statement)
		if len(parts) < 4 || strings.ToUpper(parts[2]) != "TO" {
			return 0, fmt.Errorf("invalid BRANCH statement: expected format 'BRANCH <source> TO <target>;'")
		}

		source := parts[1]
		target := strings.TrimRight(parts[3], "\x00")
		target = strings.TrimSpace(strings.TrimSuffix(target, ";"))

		file, err := os.Open(source + ".db")
		if err != nil {
			return 0, fmt.Errorf("failed to open source database '%s.db': %v", source, err)
		}
		defer file.Close()

		newFile, err := os.Create(target + ".db")
		if err != nil {
			absPath, _ := filepath.Abs(target + ".db")
			return 0, fmt.Errorf("failed to create target database '%s' at '%s': %v", target+".db", absPath, err)
		}
		defer newFile.Close()

		_, err = io.Copy(newFile, file)
		if err != nil {
			return 0, fmt.Errorf("failed to copy data from '%s.db' to '%s.db': %v", source, target, err)
		}

		return 1, nil

	case "SELECT":
		rows, err := db.Query(statement)
		if err != nil {
			return 0, err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return 0, err
		}
		SendRowDescription(w, cols)

		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			err = rows.Scan(valuePtrs...)
			if err != nil {
				return 0, err
			}
			row := make([]string, len(cols))

			for i, val := range values {
				if b, ok := val.([]byte); ok {
					row[i] = string(b) // TEXT or BLOB
				} else if s, ok := val.(string); ok {
					row[i] = s // TEXT
				} else if n, ok := val.(int64); ok {
					row[i] = strconv.FormatInt(n, 10) // INTEGER
				} else if f, ok := val.(float64); ok {
					row[i] = strconv.FormatFloat(f, 'f', -1, 64) // REAL
				} else if val == nil {
					row[i] = "NULL" // NULL
				} else {
					row[i] = fmt.Sprintf("%v", val) // fallback, should rarely happen
				}
			}
			SendDataRow(w, row)
			rowCount++

		}

		return rowCount, nil

	case "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER":
		res, err := db.Exec(statement)
		if err != nil {
			return 0, err
		}
		rowsAffected, _ := res.RowsAffected()
		return int(rowsAffected), nil

	default:
		return 0, fmt.Errorf("unsupported SQL command: %s", command)
	}
}

func HandleStatement(conn net.Conn, w io.Writer) (string, error) {
	typeBuffer := make([]byte, 1)

	io.ReadFull(conn, typeBuffer)

	msgType := typeBuffer[0]

	if msgType != MsgQuery {
		HandleError(w, fmt.Errorf("unsupported message type: %c", msgType))
		return "", fmt.Errorf("unsupported message type: %c", msgType)
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
