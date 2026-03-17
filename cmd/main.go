package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ReplicaUrls []string

type ReplicaConn struct {
	Conn     net.Conn
	Region   string
	LastUsed time.Time
}

var Connections map[string]map[string]*ReplicaConn

func main() {
	Connections = make(map[string]map[string]*ReplicaConn)

	port := flag.Int("port", 5433, "Port Number")
	replicas := flag.String("replicas", "", "replica regions")
	flag.Parse()
	addr := fmt.Sprintf(":%d", *port)
	if *replicas != "" {
		ReplicaUrls = strings.Split(*replicas, ",")
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
			fmt.Println(err)
			continue
		}
		go handleConnection(conn)

	}

}
