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

	return db, nil
}
