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
- CGO enabled (`CGO_ENABLED=1`) — required by [go-sqlite3](https://github.com/mattn/go-sqlite3)
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

Use any PostgreSQL client. The database name becomes the SQLite file:

```bash
# Connects to (or creates) myapp.db
psql -h localhost -p 5433 -d myapp

# Run queries as usual
psql> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
psql> INSERT INTO users (name) VALUES ('alice');
psql> SELECT * FROM users;

# Branch a database
psql> BRANCH myapp TO myapp_copy;
```

## How It Works

1. Client connects via TCP and negotiates the PostgreSQL startup sequence (SSL rejection, authentication)
2. The `database` parameter from the connection maps to a SQLite file (`<database>.db`) opened with WAL mode
3. SQL statements arrive as simple query messages (`Q`), get parsed and executed against SQLite
4. Results are encoded back into PostgreSQL wire format (RowDescription, DataRow, CommandComplete)
5. The custom `BRANCH` command copies the underlying `.db` file to create a snapshot

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `5433` | TCP port to listen on |
| `-replicas` | (none) | Comma-separated replica addresses |

## Supported Commands

`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ALTER`, `BEGIN`, `COMMIT`, `ROLLBACK`, `BRANCH`
