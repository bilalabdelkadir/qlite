# QLite

A PostgreSQL wire protocol proxy that runs SQLite databases underneath. Connect with any PostgreSQL client (`psql`, ORMs, drivers) and QLite translates the protocol to SQLite — giving you per-connection tenant isolation with zero configuration.

## Features

- **PostgreSQL wire protocol** — speaks the Postgres frontend/backend protocol, so standard clients just work
- **Multi-tenant** — the database name in the connection string maps to a separate SQLite file (`<name>.db`)
- **`BRANCH` command** — copy a database to a new name: `BRANCH source TO target;`
- **Transaction awareness** — tracks `BEGIN`/`COMMIT`/`ROLLBACK` to route queries correctly
- **Read replica support** — optional `-replicas` flag for read scaling with async write replication
- **Streaming results** — rows are sent to the client as they're read from SQLite, not buffered in memory
- **Buffered I/O** — all wire protocol writes go through a buffered writer, minimizing syscalls

## Prerequisites

- Go 1.25+
- CGO enabled (`CGO_ENABLED=1`) — required by [go-libsql](https://github.com/tursodatabase/go-libsql)
- A C compiler (gcc/clang)

## Installation

```bash
git clone https://github.com/bilalabdelkadir/qlite.git
cd qlite
go build -o qlite ./cmd
```

## Usage

```bash
# Start on default port 5433
./qlite

# Custom port
./qlite -port 5432

# With replicas
./qlite -replicas "localhost:5434,localhost:5435"
```

## Connecting

QLite only supports the **simple query protocol**. Some clients use the extended query protocol by default and must be configured to use simple mode.

Connection URL format:

```
postgres://postgres@localhost:5433/mydb
```

The database name in the URL (`mydb`) maps to a SQLite file (`mydb.db`). The username can be anything — authentication is not enforced.

### psql

Works out of the box (uses simple protocol by default):

```bash
psql "postgres://postgres@localhost:5433/mydb"
```

### pgx (Go)

pgx defaults to the extended query protocol. You **must** set simple protocol mode:

```go
config, err := pgx.ParseConfig("postgres://postgres@localhost:5433/mydb")
if err != nil {
    log.Fatal(err)
}
config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

conn, err := pgx.ConnectConfig(context.Background(), config)
```

### node-postgres (JavaScript)

Works by default — `pg` uses the simple protocol for `client.query()` with string arguments:

```js
const client = new Client({ connectionString: 'postgres://postgres@localhost:5433/mydb' })
await client.connect()
await client.query('SELECT * FROM users')
```

### psycopg2 (Python)

Works by default:

```python
conn = psycopg2.connect("host=localhost port=5433 dbname=mydb user=postgres")
cur = conn.cursor()
cur.execute("SELECT * FROM users")
```

## Architecture

```
                         +---------------------------------------------+
                         |              QLite Proxy                     |
                         |                                              |
psql ------+             |  +-----------+    +-------------------+     |
           |  Postgres   |  | Protocol  |    |    Executor       |     |    +--------------+
pgx -------+- wire ------+->| Handler   +--->|                   +-----+--->| myapp.db     |
           |  protocol   |  | (Q msgs)  |    | SELECT,INSERT,... |     |    | (SQLite/WAL) |
node-pg ---+             |  +-----------+    +-------------------+     |    +--------------+
                         |                                              |
                         |        | writes replicated async             |
                         |        v                                     |
                         |  +---------------+    Postgres wire          |
                         |  | Replica Conn  +------------------+       |
                         |  | Pool          |                  |       |
                         |  +---------------+                  |       |
                         |                                     |       |
                         +-------------------------------------+-------+
                                                               |
                                                               v
                                                        +-------------+
                                                        | Replica     |
                                                        | QLite nodes |
                                                        +-------------+
```

### Query lifecycle

1. Client connects via Postgres wire protocol
2. QLite handles the startup handshake (SSL rejection, AuthenticationOk, ParameterStatus)
3. The database name determines which `.db` file is opened (e.g. `myapp` -> `myapp.db`)
4. For each query:
   - Read the simple query (`Q`) message from the wire
   - Parse the command type (`SELECT`, `INSERT`, etc.)
   - Execute against the tenant's SQLite database via libSQL
   - Stream results back as Postgres wire messages (`RowDescription`, `DataRow`, `CommandComplete`)
5. All writes go through a `bufio.Writer` — the full response is batched and flushed in one syscall with `ReadyForQuery`

### Multi-tenancy

The database name from the connection string maps to a separate SQLite file. Each file is fully isolated — its own schema, data, and WAL.

```
Connection: postgres://user@host/myapp      -> myapp.db
Connection: postgres://user@host/analytics  -> analytics.db
Connection: postgres://user@host/myapp      -> myapp.db (cached, shared pool)
```

Database handles are cached globally. Multiple connections to the same database share a `*sql.DB` connection pool. The first connection to a new database triggers eager initialization via `db.Ping()` so the first query doesn't pay the cold start cost.

### BRANCH command

Copy a database to a new name using a file-level copy:

```sql
BRANCH myapp TO myapp_backup;
```

This creates `myapp_backup.db` as a full copy of `myapp.db`. The source remains unchanged. The copy is a point-in-time snapshot.

## Read Replicas

QLite can forward queries to replica QLite instances for read scaling. Replicas are other QLite processes (potentially on different machines) that the primary connects to using the same Postgres wire protocol.

### How it works

- **Writes** (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, etc.) execute on the primary first, then get forwarded asynchronously to all replicas. The client gets a response as soon as the primary completes — it does not wait for replicas.
- **Reads** (`SELECT`, `PRAGMA`, `EXPLAIN`) outside a transaction are routed to a replica using round-robin selection. The response is proxied back to the client directly.
- **Transactions** — any query inside a `BEGIN`/`COMMIT` block runs on the primary, even if it's a `SELECT`.

### Setup

Start a replica QLite instance:

```bash
./qlite -port 5434
```

Start the primary with the replica address:

```bash
# Single replica
./qlite -port 5433 -replicas "localhost:5434"

# Multiple replicas
./qlite -port 5433 -replicas "localhost:5434,localhost:5435"
```

Clients connect to the primary as usual. Replica routing is transparent.

### Replica limitations

- Replication is **asynchronous** — replicas may lag behind the primary
- Read routing uses **round-robin** (no weighted or latency-based balancing)
- Replica connections are cached per database per URL, but there is no health checking
- If a replica is down, read queries will fail rather than falling back to the primary

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `5433` | TCP port to listen on |
| `-replicas` | (none) | Comma-separated replica addresses |

### SQLite configuration

Each database is opened with:
- `_busy_timeout=5000` — waits up to 5 seconds on lock contention
- WAL mode enabled by default (via libSQL)
- Eager connection warmup via `db.Ping()` on first access

## Supported Commands

| Command | Behavior |
|---|---|
| `SELECT` | Execute and stream results to client |
| `INSERT`, `UPDATE`, `DELETE` | Execute and return rows affected |
| `CREATE`, `DROP`, `ALTER` | Execute DDL statement |
| `BEGIN`, `COMMIT`, `ROLLBACK` | Track transaction state for replica routing |
| `SET` | Accepted as no-op (returns success without executing) |
| `BRANCH source TO target;` | Copy database file |
| `PRAGMA`, `EXPLAIN` | Routed to replica if configured; unsupported locally |

## Performance

QLite is optimized for low-latency query execution:

- **Buffered I/O** — all protocol writes go through a 4KB `bufio.Writer`, batching ~20 small writes per query into 1-2 syscalls
- **No reflection in hot path** — `binary.Write` (which uses reflection) replaced with direct `binary.BigEndian.PutUint32/PutUint16` encoding
- **Type-aware value conversion** — `strconv.FormatInt`/`FormatFloat` used instead of `fmt.Sprintf("%v", val)` for SQLite result values
- **Streaming results** — rows sent to the client as they're scanned from SQLite, not buffered in memory
- **Eager connection warmup** — `db.Ping()` on first database access prevents cold start latency on the first query

### Benchmark

Using `SELECT 1` with 2 clients x 500 queries:

| Metric | QLite | PostgreSQL |
|---|---|---|
| Avg latency | ~750us | ~210us |
| QPS | ~1,300 | ~4,700 |
| Ratio | ~3.6x slower | baseline |

## Wire Protocol Details

**Supported protocol:** Simple Query only (message type `Q`)

**Backend messages sent:**

| Message | Type byte | Description |
|---|---|---|
| AuthenticationOk | `R` | Always sent (no real auth) |
| ParameterStatus | `S` | `standard_conforming_strings`, `client_encoding`, `server_version`, `integer_datetimes` |
| RowDescription | `T` | Column names and metadata |
| DataRow | `D` | Single row of query results |
| CommandComplete | `C` | Query finished with row count |
| ErrorResponse | `E` | Error with severity and message |
| ReadyForQuery | `Z` | Server ready, includes transaction status (`I` or `T`) |

**Type system:** All columns are reported as TEXT (OID 25) regardless of actual SQLite type. Values are converted to strings before transmission.

## Current Limitations

| Limitation | Details |
|---|---|
| **Extended query protocol** | No Parse/Bind/Execute messages. Only the simple query (`Q`) protocol is handled. Clients that default to extended protocol (like pgx) must be configured for simple mode. |
| **Authentication** | All connections accepted. Credentials are ignored. |
| **SSL/TLS** | Always rejected. All connections are unencrypted. |
| **Prepared statements** | Not supported (requires extended query protocol). |
| **Parameterized queries** | Not supported (requires extended query protocol). |
| **Data types** | All columns reported as `text` (OID 25). Values cast to strings. |
| **COPY protocol** | Not supported. |
| **NOTIFY/LISTEN** | Not supported. |
| **Cursors** | Not supported. |
| **Query cancellation** | Not handled. |
| **Graceful shutdown** | No connection draining on server stop. |
| **Multiple statements** | Only one statement per query message is executed. |
| **PRAGMA/EXPLAIN locally** | Only work when replicas are configured (routed to replica). Not handled by the local executor. |
