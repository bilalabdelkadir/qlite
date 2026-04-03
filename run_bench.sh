#!/bin/bash

NUM_RUNS=5
OUTPUT_FILE="benchmark_results.txt"
QLITE_PORT=5433
PG_PORT=5434
BENCHMARKS="select_1 insert select_rows"

> "$OUTPUT_FILE"

# start postgres via docker
echo "Starting Postgres on port $PG_PORT..."
docker compose up -d --wait
if ! nc -z localhost "$PG_PORT" 2>/dev/null; then
    echo "ERROR: Postgres failed to start on port $PG_PORT"
    docker compose down
    exit 1
fi
echo "Postgres ready."

# build qlite
echo "Building qlite..."
go build -o qlite ./cmd || { docker compose down; exit 1; }

for i in $(seq 1 "$NUM_RUNS"); do
    echo "===== Run #$i =====" | tee -a "$OUTPUT_FILE"

    for bench in $BENCHMARKS; do
        # fresh qlite process per benchmark to avoid CGO memory buildup
        rm -f benchdb.db

        ./qlite &
        QLITE_PID=$!

        until nc -z localhost "$QLITE_PORT" 2>/dev/null; do
            sleep 0.2
        done

        go run bench/main.go -bench "$bench" >> "$OUTPUT_FILE" 2>&1

        # check if qlite survived
        if ! kill -0 "$QLITE_PID" 2>/dev/null; then
            echo "  [WARN] qlite crashed during $bench" >> "$OUTPUT_FILE"
        else
            kill "$QLITE_PID" 2>/dev/null
            wait "$QLITE_PID" 2>/dev/null
        fi
    done

    echo "" >> "$OUTPUT_FILE"
done

# cleanup
docker compose down
echo "Done. Results in $OUTPUT_FILE"
