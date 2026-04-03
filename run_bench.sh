#!/bin/bash
#
# QLite Benchmark — runs 3 workloads against QLite and Postgres side by side.
# One command: ./run_bench.sh
#

QLITE_PORT=5433
PG_PORT=5434
QLITE_URL="postgres://postgres@localhost:$QLITE_PORT/benchdb"
PG_URL="postgres://bench:bench@localhost:$PG_PORT/benchdb"
OUTPUT_FILE="benchmark_results.txt"
NUM_RUNS=3

> "$OUTPUT_FILE"

cleanup() {
    kill "$QLITE_PID" 2>/dev/null
    wait "$QLITE_PID" 2>/dev/null
    docker compose down 2>/dev/null
    rm -f benchdb.db
}
trap cleanup EXIT

# --- start postgres ---
echo "Starting Postgres on port $PG_PORT..."
docker compose up -d --wait
if ! nc -z localhost "$PG_PORT" 2>/dev/null; then
    echo "ERROR: Postgres failed to start"
    exit 1
fi
echo "Postgres ready."

# --- build qlite ---
echo "Building qlite..."
go build -o qlite ./cmd || exit 1

start_qlite() {
    rm -f benchdb.db
    ./qlite -port "$QLITE_PORT" &
    QLITE_PID=$!
    sleep 1
}

stop_qlite() {
    kill "$QLITE_PID" 2>/dev/null
    wait "$QLITE_PID" 2>/dev/null
}

run_bench() {
    local label=$1 query=$2 setup=$3 clients=${4:-2} queries=${5:-500}

    echo "--- $label ---" | tee -a "$OUTPUT_FILE"

    # qlite
    start_qlite
    if [ -n "$setup" ]; then
        go run bench/main.go \
            -url="$QLITE_URL" -label="QLite" \
            -query="$query" -clients="$clients" -queries="$queries" \
            -setup="$setup" >> "$OUTPUT_FILE" 2>&1
    else
        go run bench/main.go \
            -url="$QLITE_URL" -label="QLite" \
            -query="$query" -clients="$clients" -queries="$queries" \
            >> "$OUTPUT_FILE" 2>&1
    fi
    stop_qlite

    # postgres
    if [ -n "$setup" ]; then
        go run bench/main.go \
            -url="$PG_URL" -label="Postgres" \
            -query="$query" -clients="$clients" -queries="$queries" \
            -setup="$setup" >> "$OUTPUT_FILE" 2>&1
    else
        go run bench/main.go \
            -url="$PG_URL" -label="Postgres" \
            -query="$query" -clients="$clients" -queries="$queries" \
            >> "$OUTPUT_FILE" 2>&1
    fi

    echo "" >> "$OUTPUT_FILE"
}

# --- benchmarks ---
for i in $(seq 1 "$NUM_RUNS"); do
    echo "===== Run #$i =====" | tee -a "$OUTPUT_FILE"

    run_bench "SELECT 1" \
        "SELECT 1"

    run_bench "INSERT" \
        "INSERT INTO bench_test (val) VALUES ('hello')" \
        "DROP TABLE IF EXISTS bench_test; CREATE TABLE bench_test (val TEXT)" \
        1 200

    run_bench "SELECT rows" \
        "SELECT * FROM bench_read WHERE id = 1" \
        "DROP TABLE IF EXISTS bench_read; CREATE TABLE bench_read (id INTEGER, val TEXT); INSERT INTO bench_read VALUES (1, 'test')" \
        2 500

    echo "" >> "$OUTPUT_FILE"
done

echo "Done. Results in $OUTPUT_FILE"
