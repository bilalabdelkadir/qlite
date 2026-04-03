package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	url := flag.String("url", "postgres://postgres@localhost:5433/benchdb", "database URL")
	query := flag.String("query", "SELECT 1", "SQL query to benchmark")
	setup := flag.String("setup", "", "SQL to run once before benchmark (e.g. CREATE TABLE)")
	clients := flag.Int("clients", 2, "number of concurrent clients")
	queries := flag.Int("queries", 500, "queries per client")
	label := flag.String("label", "", "benchmark label for output")
	flag.Parse()

	ctx := context.Background()

	config, err := pgx.ParseConfig(*url)
	if err != nil {
		log.Fatalf("failed to parse config: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	// run setup queries if provided (split on ; for multiple statements)
	if *setup != "" {
		conn, err := pgx.ConnectConfig(ctx, config)
		if err != nil {
			log.Fatalf("setup connection failed: %v", err)
		}
		for _, stmt := range strings.Split(*setup, ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := conn.Exec(ctx, stmt); err != nil {
				log.Fatalf("setup query failed: %v", err)
			}
		}
		conn.Close(ctx)
	}

	total := *clients * *queries
	if *label != "" {
		fmt.Printf("[%s] ", *label)
	}
	fmt.Printf("%d clients × %d queries = %d total\n", *clients, *queries, total)

	latencies := make([]time.Duration, 0, total)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(*clients)

	start := time.Now()

	for c := 0; c < *clients; c++ {
		go func(id int) {
			defer wg.Done()
			conn, err := pgx.ConnectConfig(ctx, config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "client %d connect failed: %v\n", id, err)
				return
			}
			defer conn.Close(ctx)

			local := make([]time.Duration, 0, *queries)
			for i := 0; i < *queries; i++ {
				t := time.Now()
				if _, err := conn.Exec(ctx, *query); err != nil {
					log.Printf("client %d query error: %v", id, err)
				}
				local = append(local, time.Since(t))
			}

			mu.Lock()
			latencies = append(latencies, local...)
			mu.Unlock()
		}(c)
	}

	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := percentile(latencies, 50)
	p95 := percentile(latencies, 95)
	p99 := percentile(latencies, 99)
	qps := float64(len(latencies)) / elapsed.Seconds()

	fmt.Printf("  p50: %v  p95: %v  p99: %v\n", p50, p95, p99)
	fmt.Printf("  QPS: %.0f  Total: %v\n\n", qps, elapsed)
}

func percentile(sorted []time.Duration, pct float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(pct/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}
