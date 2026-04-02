package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {

	ctx := context.Background()

	dbUrl := "postgres://postgres@localhost:5433/benchdb"
	// dbUrl := "postgresql://admin:2334@localhost:5432/mini_inventory"
	numClients := 2
	queriesPerClient := 500
	sqlQuery := "SELECT 1"

	fmt.Printf("Running benchmark: %d clients × %d queries each = %d total queries\n",
		numClients, queriesPerClient, numClients*queriesPerClient)

	startTime := time.Now()

	var wg sync.WaitGroup
	wg.Add(numClients)

	for c := 0; c < numClients; c++ {
		go func(clientID int) {
			config, err := pgx.ParseConfig(dbUrl)
			if err != nil {
				log.Fatalf("failed to parse config: %v", err)
			}
			config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
			conn, err := pgx.ConnectConfig(context.Background(), config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
				log.Fatalf("failed to connect: %v", err)
			}
			defer wg.Done()
			defer conn.Close(context.Background())
			for i := 0; i < queriesPerClient; i++ {
				_, err := conn.Exec(ctx, sqlQuery)
				if err != nil {
					log.Printf("Client %d query error: %v", clientID, err)
				}
			}
		}(c)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	totalQueries := numClients * queriesPerClient
	avgLatency := elapsed / time.Duration(totalQueries)
	qps := float64(totalQueries) / elapsed.Seconds()

	fmt.Println("Benchmark finished!")
	fmt.Println("Total time:", elapsed)
	fmt.Println("Average latency per query:", avgLatency)
	fmt.Printf("QPS: %.2f\n", qps)

}
