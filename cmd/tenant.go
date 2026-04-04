package main

import (
	"database/sql"
	"fmt"
)

func HandleTenantDb(dbName string) (*sql.DB, error) {
	dbFile := fmt.Sprintf("file:./%s.db?_busy_timeout=5000", dbName)
	db, err := sql.Open("libsql", dbFile)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}

	var mode string
	err = db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode)
	if err != nil {
		db.Close()
		return nil, err
	}
	if mode != "wal" {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode, got: %s", mode)
	}

	return db, nil
}
