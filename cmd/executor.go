package main

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func ExtractCommand(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}

	parts := strings.Fields(statement)
	command := strings.ToUpper(parts[0])

	return command

}

func HandleExecute(db *sql.DB, statement string) (columns []string, rows [][]string, rowsAffected int, err error) {
	command := ExtractCommand(statement)

	switch command {
	case "BRANCH":
		fmt.Printf("raw statement bytes: %q\n", statement)
		parts := strings.Fields(statement)
		if len(parts) < 4 || strings.ToUpper(parts[2]) != "TO" {
			return nil, nil, 0, fmt.Errorf("invalid BRANCH statement: expected format 'BRANCH <source> TO <target>;'")
		}

		source := parts[1]
		target := strings.TrimRight(parts[3], "\x00")
		target = strings.TrimSpace(strings.TrimSuffix(target, ";"))

		file, err := os.Open(source + ".db")
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to open source database '%s.db': %v", source, err)
		}
		defer file.Close()

		newFile, err := os.Create(target + ".db")
		if err != nil {
			absPath, _ := filepath.Abs(target + ".db")
			return nil, nil, 0, fmt.Errorf("failed to create target database '%s' at '%s': %v", target+".db", absPath, err)
		}
		defer newFile.Close()

		_, err = io.Copy(newFile, file)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to copy data from '%s.db' to '%s.db': %v", source, target, err)
		}

		return nil, nil, 1, nil

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
