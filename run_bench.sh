#!/bin/bash

NUM_RUNS=10
OUTPUT_FILE="benchmark_results.txt"

> $OUTPUT_FILE

for i in $(seq 1 $NUM_RUNS); do
    echo "===== Run #$i =====" >> $OUTPUT_FILE

    rm -f benchdb.db

    ./qlite &
    QLITE_PID=$!

    echo "Waiting for QLite to be ready on port 5433..."
    until nc -z localhost 5433; do
        sleep 0.5
    done

    echo "QLite is ready!"

    go run bench/main.go | grep -E "Average latency|QPS" >> $OUTPUT_FILE

    kill $QLITE_PID
    wait $QLITE_PID
    echo "QLite stopped" >> $OUTPUT_FILE
    echo "" >> $OUTPUT_FILE
done
