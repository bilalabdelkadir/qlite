package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

var replicaUrls []string

type replicaConn struct {
	Conn     net.Conn
	Region   string
	LastUsed time.Time
}

// map[dbName]map[replicaURL]*ReplicaConn
var connections map[string]map[string]*replicaConn
var connectionsMu sync.RWMutex

func main() {
	connections = make(map[string]map[string]*replicaConn)

	port := flag.Int("port", 5433, "Port Number")
	replicas := flag.String("replicas", "", "replica regions")
	flag.Parse()
	addr := fmt.Sprintf(":%d", *port)
	if *replicas != "" {
		replicaUrls = strings.Split(*replicas, ",")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("TCP server listening on %s", addr)

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)

	}

}
