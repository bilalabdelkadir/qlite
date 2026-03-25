# QLite

A PostgreSQL wire protocol proxy that runs SQLite databases underneath. Connect with any PostgreSQL client (`psql`, ORMs, drivers) and QLite translates the protocol to SQLite — giving you per-connection tenant isolation with zero configuration.

## Features

- **PostgreSQL wire protocol** — speaks the Postgres frontend/backend protocol, so standard clients just work
- **Multi-tenant** — the database name in the connection string maps to a separate SQLite file (`<name>.db`), each with WAL mode and busy timeout configured
- **`BRANCH` command** — copy a database to a new name: `BRANCH source TO target;`
- **Transaction awareness** — tracks `BEGIN`/`COMMIT`/`ROLLBACK` to route queries correctly
- **Read replica infrastructure** — optional `--replicas` flag to specify replica regions (read routing in progress)

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

# With replica regions
./qlite -replicas "us-east-1:5434,eu-west-1:5435"
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
dbUrl := "postgres://postgres@localhost:5433/mydb"
config, err := pgx.ParseConfig(dbUrl)
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
                         ┌─────────────────────────────────────────────┐
                         │              QLite Proxy                    │
                         │                                             │
psql ──────┐             │  ┌───────────┐    ┌───────────────────┐    │
           │  Postgres   │  │ Protocol  │    │    Executor       │    │    ┌──────────────┐
pgx ───────┼─ wire ──────┼─►│ Handler   ├───►│                   ├────┼───►│ myapp.db     │
           │  protocol   │  │ (Q msgs)  │    │ SELECT,INSERT,... │    │    │ (SQLite/WAL) │
node-pg ───┘             │  └───────────┘    └───────────────────┘    │    └──────────────┘
                         │                                             │
                         │        │ writes replicated async            │
                         │        ▼                                    │
                         │  ┌───────────────┐    Postgres wire         │
                         │  │ Replica Conn  ├────────────────────┐    │
                         │  │ Pool          │                    │    │
                         │  └───────────────┘                    │    │
                         │                                       │    │
                         └───────────────────────────────────────┼────┘
                                                                 │
                                                                 ▼
                                                          ┌─────────────┐
                                                          │ Replica     │
                                                          │ QLite nodes │
                                                          └─────────────┘
```

Clients connect using the Postgres wire protocol. QLite handles the startup handshake (SSL rejection, AuthenticationOk, ParameterStatus), then enters a loop reading simple query (`Q`) messages. Each query is parsed, executed against a per-tenant SQLite file via libSQL, and the results are encoded back as Postgres wire messages (RowDescription, DataRow, CommandComplete). The database name in the connection string determines which `.db` file is opened — connecting as `myapp` reads and writes `myapp.db`.

## Read Replicas

QLite can forward queries to replica QLite instances for read scaling. Replicas are other QLite processes (potentially on different machines) that the primary connects to using the same Postgres wire protocol internally.

### How it works

- **Writes** (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, etc.) execute on the primary first, then get forwarded asynchronously to all replicas. The client gets a response as soon as the primary completes — it does not wait for replicas.
- **Reads** (`SELECT`, `PRAGMA`, `EXPLAIN`) outside a transaction are routed to a replica instead of the primary. The response is proxied back to the client directly.
- **Transactions** — any query inside a `BEGIN`/`COMMIT` block runs on the primary, even if it's a `SELECT`.

### Setup

Start a replica QLite instance:

```bash
# Replica on port 5434
./qlite -port 5434
```

Start the primary with the replica address:

```bash
# Primary on port 5433, forwarding to replica on 5434
./qlite -port 5433 -replicas "localhost:5434"

# Multiple replicas
./qlite -port 5433 -replicas "localhost:5434,localhost:5435"
```

Clients connect to the primary as usual. Replica routing is transparent:

```bash
psql "postgres://postgres@localhost:5433/mydb"
```

### Replica limitations

- Replication is **asynchronous** — replicas may lag behind the primary
- Read routing always uses the **first replica** in the list (no load balancing yet)
- Replica connections are cached per database per URL, but there is no health checking or automatic reconnection
- If a replica is down, read queries will fail rather than falling back to the primary

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `5433` | TCP port to listen on |
| `-replicas` | (none) | Comma-separated replica addresses |

## Supported Commands

`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ALTER`, `BEGIN`, `COMMIT`, `ROLLBACK`, `BRANCH`, `SET`, `PRAGMA`, `EXPLAIN`

`SET` is accepted but treated as a no-op (returns success without executing). `PRAGMA` and `EXPLAIN` are routed as read queries.

## Current Limitations

QLite is early-stage. The following are **not yet supported**:

| Limitation | Details |
|---|---|
| **Extended query protocol** | No Parse/Bind/Execute messages. Only the simple query (`Q`) protocol is handled. Clients that default to extended protocol (like pgx) must be configured for simple mode. |
| **Authentication** | All connections are accepted regardless of credentials. User and password fields are ignored. |
| **SSL/TLS** | SSL requests are always rejected. All connections are unencrypted. |
| **Prepared statements** | Not supported (requires extended query protocol). |
| **Parameterized queries** | Not supported (requires extended query protocol). |
| **Data types** | All columns are reported as `text` (OID 25) regardless of actual SQLite type. Values are cast to strings. |
| **COPY protocol** | Bulk loading via `COPY` is not supported. |
| **NOTIFY/LISTEN** | Async notification channels are not supported. |
| **Cursors** | No cursor-based result fetching. |
| **Query cancellation** | Cancel requests are not handled. |
| **Graceful shutdown** | No connection draining on server stop. |
| **Multiple result sets** | Only one statement per query message is executed. |
