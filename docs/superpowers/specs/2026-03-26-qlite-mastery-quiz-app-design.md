# QLite Mastery -- Interactive Quiz App Design

## Context

Bilal has an upcoming call (~2026-03-28) with Rafael Umann, CEO of Azion (Cloudflare competitor), who is interested in hiring/collaborating to build a Postgres-compatible, scale-to-zero, write-scalable database for Azion's edge network -- based on Bilal's qlite project. Bilal needs to understand his codebase inside and out, be able to explain every architectural decision, and articulate how the project can evolve to meet Azion's needs.

This app is an interactive learning tool with visualizations, quizzes, and interview scenario practice covering all 8 critical topics over 8 days of preparation.

## Architecture

**Type:** Multi-page static site (vanilla HTML/CSS/JS, zero dependencies)

### File Structure

```
quiz-app/
├── index.html                      # Dashboard + progress tracker
├── topic-1-wire-protocol.html      # Day 1: PostgreSQL Wire Protocol
├── topic-2-connection-lifecycle.html # Day 2: Connection Lifecycle
├── topic-3-query-pipeline.html     # Day 3: Query Execution Pipeline
├── topic-4-multi-tenancy.html      # Day 4: Multi-Tenancy
├── topic-5-branch-command.html     # Day 5: BRANCH Command
├── topic-6-replicas.html           # Day 6: Read Replica Architecture
├── topic-7-sqlite-libsql.html      # Day 7: SQLite/libSQL & Write Concurrency
├── topic-8-azion-edge.html         # Day 8: Scale-to-Zero & Edge Integration
├── styles.css                      # Shared dark theme, animations, layout
├── quiz-engine.js                  # Quiz logic, scoring, progress (LocalStorage)
└── animations.js                   # Protocol flow animations, code trace engine
```

### Why This Architecture

- **No build step** -- open `index.html` in any browser, start learning
- **One topic per file** -- matches the 8-day study plan, keeps files manageable
- **Shared JS modules** -- quiz logic and animation engines reused across all pages
- **LocalStorage** -- simple progress persistence, no backend needed
- **Portable** -- zip and use anywhere

## Page Structure

### Dashboard (`index.html`)

- App title: "QLite Mastery -- 8 Days to Azion"
- 8 topic cards in a grid, each showing:
  - Topic number and title
  - Day recommendation (Day 1, Day 2, etc.)
  - Completion percentage (from LocalStorage)
  - Color-coded status: not started / in progress / mastered
- Simple "Readiness" bar at top -- average of all topic scores
- Click a card to navigate to that topic page

### Topic Pages (3 Phases per page)

Each topic page has a tab/section navigation for its 3 phases. User progresses through them in order but can jump between phases.

#### Phase 1: Learn (Visual Explanations)

Two visualization types used throughout:

**Animated Protocol Flows:**
- SVG-based diagrams with Client and Server columns
- Messages animate between them as colored blocks
- Each step has a "Next" button to advance
- Current step is highlighted; previous steps dim
- Tooltip/sidebar shows the byte-level breakdown of each message
- Built with CSS animations + JS state machine in `animations.js`

**Code Trace Walkthroughs:**
- Displays actual qlite Go source code in a syntax-highlighted block
- Current execution line highlighted with a glow effect
- Side panel shows variable state / data at that point
- "Step" button advances to next relevant line
- Maps to real file paths (e.g., "server.go:87 -- handleConnection()")

#### Phase 2: Quiz (Test Recall)

Three question types managed by `quiz-engine.js`:

**Multiple Choice:**
- 4 options, one correct
- Immediate feedback (green/red highlight)
- Wrong answers show brief explanation

**Order the Steps:**
- Drag-and-drop or click-to-order
- User arranges items (e.g., handshake message sequence)
- Submit to check, incorrect items highlighted

**Fill in the Blank:**
- Statement with blanks (e.g., "The message type byte for RowDescription is ___")
- Text input, case-insensitive matching
- Reveals answer on submit

Scoring: percentage correct per topic, saved to LocalStorage.

#### Phase 3: Rafael Asks... (Scenario Practice)

- Shows a question styled as if Rafael is asking it in a conversation
- Large text area for Bilal to type his answer
- "Reveal Model Answer" button shows a strong example answer
- Key points checklist appears -- Bilal self-rates which points he covered
- Self-rating saved to progress

## Topic Content

### Day 1: PostgreSQL Wire Protocol

**Learn:**
- Animated diagram: binary message format (1-byte type | 4-byte length | payload)
- Byte inspector: hover over each byte in a RowDescription message to see its meaning
- Code trace: `protocol.go` -- `SendRowDescription()`, `SendDataRow()`, `SendCommandComplete()`, `HandleError()`
- Key constants: message types (R, S, T, D, C, Z, E), OID 25 (text), format codes

**Quiz (8-10 questions):**
- "What is the message type byte for ErrorResponse?" → E
- "How many bytes is the message length field?" → 4
- "What OID does qlite use for all column types?" → 25 (text)
- "Does the length field include the type byte?" → No, only includes itself + payload
- Order: fields within a RowDescription column descriptor
- Fill in: "The protocol version for Postgres v3.0 is ___" → 196608

**Rafael Asks:**
- "Why did you choose the Postgres wire protocol instead of HTTP or a custom protocol?"
  - Key points: ecosystem compatibility, any Postgres driver/ORM works, no client-side changes needed, battle-tested protocol
- "What Postgres features can't you support with this approach?"
  - Key points: extended query protocol, prepared statements, COPY, NOTIFY/LISTEN, type-specific OIDs, SSL/TLS (not yet)
- "How complex was implementing the wire protocol?"
  - Key points: simple query protocol is straightforward, binary encoding in Go is clean with encoding/binary, the hard part is completeness not complexity

### Day 2: Connection Lifecycle

**Learn:**
- Animated flow: TCP connect → SSL request → reject 'N' → startup message → parse user/database → AuthenticationOk (R+0) → 4x ParameterStatus (S) → ReadyForQuery loop (Z+'I'/'T')
- Code trace: `server.go` -- `handleConnection()`, `HandleSslRequest()`, `HandleStartup()`
- Highlight: how `isInTransaction` flag toggles on BEGIN/COMMIT/ROLLBACK

**Quiz (8-10 questions):**
- "What byte does the server send to reject SSL?" → N
- "What is the SSL request code?" → 80877103
- "Name the 4 ParameterStatus values sent during startup" → standard_conforming_strings, client_encoding, server_version, integer_datetimes
- Order: startup handshake message sequence
- "What does 'Z' + 'T' mean?" → ReadyForQuery, in transaction

**Rafael Asks:**
- "Walk me through what happens from the moment psql connects to your server"
- "How would you add TLS support?"
- "What happens if a client sends an extended query message?"

### Day 3: Query Execution Pipeline

**Learn:**
- Animated flow: Q message → `HandleStatement()` reads type+length+payload → `ExtractCommand()` → routing decision → `HandleExecute()` → SQLite via libsql → encode results as T+D+C messages
- Code trace: `executor.go` -- `HandleStatement()`, `ExtractCommand()`, `IsReadQuery()`, `HandleExecute()`
- Diagram: command routing switch (SET→no-op, BRANCH→file copy, SELECT→query, DML→exec)

**Quiz (8-10 questions):**
- "What 3 prefixes does IsReadQuery check for?" → SELECT, PRAGMA, EXPLAIN
- "How are column values converted before sending?" → fmt.Sprintf("%v", val)
- "What is the CommandComplete tag format for INSERT?" → "INSERT 0 {count}"
- Fill in: "The SET command is handled as a ___" → no-op

**Rafael Asks:**
- "What's the performance overhead of this translation layer?"
- "How would you add prepared statement support?"
- "What SQL differences between Postgres and SQLite cause problems?"

### Day 4: Multi-Tenancy

**Learn:**
- Diagram: connection string `postgres://user@host/myapp` → startup message → `database=myapp` → `HandleTenantDb("myapp")` → `file:./myapp.db?_busy_timeout=5000`
- Code trace: `tenant.go` -- `HandleTenantDb()`
- Visual: file system showing separate .db files per tenant

**Quiz (6-8 questions):**
- "What SQLite file does database name 'customers' map to?" → ./customers.db
- "What is the busy timeout value?" → 5000ms
- "At what level are tenants isolated?" → File level (separate .db files)
- "What driver does qlite use?" → github.com/tursodatabase/go-libsql

**Rafael Asks:**
- "How would this scale to 10,000 tenants on a single node?"
- "What's your isolation story -- can one tenant affect another's performance?"
- "How does this compare to Postgres schemas for multi-tenancy?"

### Day 5: BRANCH Command

**Learn:**
- Animated flow: `BRANCH testdb TO staging` → parse parts → `os.Open("testdb.db")` → `os.Create("staging.db")` → `io.Copy()` → new isolated database
- Diagram: comparing BRANCH to git branching -- instant snapshot of entire database state

**Quiz (6-8 questions):**
- "What's the BRANCH syntax?" → BRANCH source TO target
- "What Go function copies the bytes?" → io.Copy
- "Is the new database linked to the source?" → No, fully independent copy
- "What happens if the source doesn't exist?" → Error returned

**Rafael Asks:**
- "How would you make BRANCH work across replicas?"
- "Could this be used for blue-green deployments?"
- "How does this compare to Postgres's pg_dump or logical replication?"

### Day 6: Read Replica Architecture

**Learn:**
- Full system diagram: Primary (writes + reads without replicas) ↔ Replicas (read-only). Show the connection pool structure: `map[dbName]map[replicaURL]*replicaConn`
- Animated flow (write): Client → primary executes → response to client → goroutines fan out to replicas (fire-and-forget)
- Animated flow (read): Client → atomic counter picks replica → forward Q message → proxy T/D/C response back
- Code trace: `server.go` -- `GetOrCreateReplicaConn()`, `sendQuery()`, `handleReadQuery()`, `handleWriteQuery()`

**Quiz (8-10 questions):**
- "What's the load balancing strategy?" → Round-robin via atomic counter
- "Are write replications synchronous or async?" → Async (goroutines, fire-and-forget)
- "What happens to SELECTs inside a BEGIN block?" → Routed to primary, not replica
- "What timeout is used for replica TCP connection?" → 5 seconds
- Order: steps in handleWriteQuery (execute local → send response → async replicate)

**Rafael Asks:**
- "What are your consistency guarantees with async replication?"
- "How would you handle a replica going down?"
- "What would it take to add synchronous replication?"

### Day 7: SQLite, libSQL & Write Concurrency

**Learn:**
- Diagram: SQLite's concurrency model (readers + 1 writer via WAL) vs libSQL's write concurrency
- WAL explainer: Write-Ahead Log -- writes go to WAL file, readers see committed state, checkpoint merges WAL back
- Diagram: Why vanilla SQLite can't scale writes → single writer lock → libSQL's solution
- Migration path: go-libsql driver already in use → swap underlying engine, CGO bridge stays the same

**Quiz (8-10 questions):**
- "What does WAL stand for?" → Write-Ahead Logging
- "How many concurrent writers does vanilla SQLite allow?" → 1
- "What Go driver does qlite use for SQLite?" → go-libsql (tursodatabase)
- "What does _busy_timeout prevent?" → "database locked" errors
- "What language is libSQL written in?" → Rust (fork of SQLite)

**Rafael Asks:**
- "How would you swap SQLite for libSQL with write concurrency in your codebase?"
- "What's your plan for handling concurrent writes at Azion's scale?"
- "How does libSQL's approach differ from CockroachDB or TiDB?"

### Day 8: Scale-to-Zero & Edge Integration

**Learn:**
- Diagram: Traditional DB (always running, connection pools) vs Scale-to-Zero (spin up on request, shut down on idle)
- Why SQLite/libSQL is perfect for edge: no connection pool warmup, file = database, instant cold start
- Competitive landscape diagram: Cloudflare D1 vs Turso vs PlanetScale vs Azion's opportunity
- Architecture sketch: How qlite could run at Azion edge nodes -- request arrives → qlite process starts → opens .db file → serves queries → scales down

**Quiz (6-8 questions):**
- "What does scale-to-zero mean?" → Resources deallocated when idle, allocated on demand
- "Why is SQLite ideal for edge deployment?" → File-based, no daemon, instant startup, small footprint
- "What is Azion's primary competitor?" → Cloudflare
- "What database does Cloudflare offer?" → D1

**Rafael Asks:**
- "How would you integrate qlite into Azion's edge network?"
- "What would full Postgres compliance look like -- what's the gap?"
- "Why should Azion build on your project instead of forking Turso directly?"
- "What's your technical roadmap for the next 6 months?"

## Shared Components

### styles.css

- **Theme:** Dark mode (matches developer aesthetic)
- **Color system:** Each topic gets an accent color for its cards and highlights
- **Typography:** Monospace for code, clean sans-serif for content
- **Animations:** CSS transitions for message flows, keyframes for step highlighting
- **Responsive:** Works on laptop screens (primary use case)

### quiz-engine.js

- `initQuiz(topicId, questions)` -- sets up quiz for a topic page
- `checkAnswer(questionId, answer)` -- validates and shows feedback
- `saveProgress(topicId, score)` -- writes to LocalStorage
- `getProgress()` -- returns all topic scores for dashboard
- Question types: `multiple-choice`, `order-steps`, `fill-blank`, `scenario`
- Scoring: percentage per topic, overall readiness percentage

### animations.js

- `ProtocolFlow(containerId, steps)` -- renders animated client/server message flow
  - Each step: `{ from, to, messageType, label, bytes }`
  - CSS animation slides message block between columns
  - "Next Step" button advances, highlights current step
- `CodeTrace(containerId, codeLines, steps)` -- renders code walkthrough
  - Each step: `{ line, highlight, sidebar }`
  - Highlights current line with glow, shows context in side panel
  - "Step" button advances through execution

## Progress Tracking

**LocalStorage schema:**
```json
{
  "qlite-mastery": {
    "topic-1": { "quizScore": 80, "scenariosDone": 2, "completed": false },
    "topic-2": { "quizScore": null, "scenariosDone": 0, "completed": false }
  }
}
```

Dashboard reads this to render:
- Per-topic completion percentage
- Color: gray (not started), blue (in progress), green (>80% score)
- Overall readiness bar (average of completed topics)

## Verification

1. Open `quiz-app/index.html` in a browser -- dashboard should render with 8 topic cards
2. Click Topic 1 -- should show Learn phase with animated protocol flow
3. Step through the animation -- messages should animate between client/server
4. Switch to Quiz phase -- answer questions, verify immediate feedback
5. Switch to Rafael Asks -- type an answer, reveal model answer, self-rate
6. Return to dashboard -- verify progress is saved and displayed
7. Refresh browser -- progress persists via LocalStorage
8. Test all 8 topics for basic functionality
