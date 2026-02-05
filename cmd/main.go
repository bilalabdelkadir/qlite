package main

import (
	"fmt"
	"log"
	"net"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	listener, err := net.Listen("tcp", ":5433")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("tcp connection is running on 5433 happy hacking")
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
