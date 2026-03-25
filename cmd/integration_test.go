package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestIntegration(t *testing.T) {

	listener, err := net.Listen("tcp", ":5499")
	if err != nil {
		t.Errorf("test integration failed to start tcp server.")
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				t.Errorf("test integration failed to listen.")
			}

			go handleConnection(conn)
		}
	}()

	time.Sleep(time.Second * 2)

	dbUrl := "postgres://postgres@localhost:5499/testingdb"
	config, err := pgx.ParseConfig(dbUrl)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(context.Background(), config)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(context.Background(), "CREATE TABLE IF NOT EXISTS users (id INTEGER, name TEXT)")
	if err != nil {
		t.Errorf("CREATE TABLE failed: %v", err)
	} else {
		t.Logf("CREATE TABLE succeeded")
	}

	// Step 2: INSERT a row
	_, err = conn.Exec(context.Background(), "INSERT INTO users VALUES (1, 'bilal')")
	if err != nil {
		t.Errorf("INSERT failed: %v", err)
	} else {
		t.Logf("INSERT succeeded")
	}

	var id string
	var name string
	err = conn.QueryRow(context.Background(), "SELECT id, name FROM users WHERE id=1").Scan(&id, &name)
	if err != nil {
		t.Errorf("SELECT failed: %v", err)
	} else {
		t.Logf("SELECT succeeded: id=%s, name=%s", id, name)
	}

	// Step 4: Validate the result
	if id != "1" || name != "bilal" {
		t.Errorf("SELECT returned wrong data: got id=%s, name=%q, want id=1, name='bilal'", id, name)
	} else {
		t.Logf("SELECT returned correct data")
	}

	// Optional: clean up table for next test run
	_, err = conn.Exec(context.Background(), "DROP TABLE users")
	if err != nil {
		t.Errorf("DROP TABLE failed: %v", err)
	} else {
		t.Logf("DROP TABLE succeeded")
	}

}
