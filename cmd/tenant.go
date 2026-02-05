package main

import (
	"database/sql"
	"fmt"
)

func HandleTenantDb(dbName string) (*sql.DB, error) {
	dbFile := fmt.Sprintf("./%s.db?_busy_timeout=5000&_journal_mode=WAL", dbName)
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	return db, nil
}
