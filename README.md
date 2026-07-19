![DBfock banner](./banner.png)

# DBfock

DBfock is a focused MySQL workspace for browsing schemas, inspecting data, running SQL, saving useful queries, and working with AI-assisted database tooling. It is inspired by database IDEs such as DBeaver and DataGrip, but keeps the interface quiet and workspace-first.

DBfock runs as a browser app through Docker or local development servers, and as a native desktop app through Wails.

## What it does

- Create, test, edit, import, export, and securely store MySQL connections.
- Browse databases, tables, views, columns, indexes, constraints, foreign keys, triggers, references, DDL, and paginated table data.
- Run SQL with query history, cancellable requests, bounded result sets, CSV/JSON/TSV export, and multiple result tabs.
- Save reusable SQL snippets and generate parameterized smart queries from selected SQL.
- Use the AI agent panel for schema-aware query explanation, improvement, and chat, with configurable OpenAI, OpenRouter, Anthropic, or Ollama providers.
- Track AI audit logs and connection-level query stats.
- Mark connections as development or production and commit or roll back pending production transactions explicitly.
- Customize themes, text scale, shortcuts, and connection colors.
- Import compatible MySQL connection metadata from DBeaver project files.

## Architecture

```text
Nuxt 4 / Vue 3 / TypeScript
        | REST
Go + Chi API -- provider registry -- MySQL (`database/sql`)
        |
SQLite application store (connections, settings, history, audit logs)

Wails v2 packages the Nuxt build and starts the local Go API in-process.
```

- **Backend:** Go 1.24, Chi, `database/sql`, `github.com/go-sql-driver/mysql`, SQLite through `modernc.org/sqlite`, and Wails v2 for desktop packaging.
- **Frontend:** Nuxt 4.4, Vue 3 Composition API, Pinia, Tailwind CSS, and CodeMirror SQL editing.
- **Provider boundary:** the `database.Provider` interface isolates database-specific discovery, data, and query behavior. MySQL is implemented today; PostgreSQL, SQL Server, or SQLite can be added behind the same HTTP surface.
- **Application data vs. managed data:** SQLite stores DBfock metadata only. It does not replace or mingle with the MySQL databases being managed.

## Directory structure

```text
backend/
  main.go                  Wails desktop entrypoint
  wails.json               Wails build configuration
  cmd/api/                 Browser/API entrypoint
  desktop/assets/          Generated Nuxt assets embedded by Wails
  internal/                API, config, DB providers, models, repository, AI, middleware
  migrations/              SQLite application migrations
frontend/
  app/
    components/ pages/ stores/ types/ composables/ assets/ layouts/
  public/branding/         Browser and desktop branding assets
banner.png                 README banner
docker-compose.yml         Browser stack
Makefile                   Common development commands
```

## Run with Docker

Requirements: Docker, Docker Compose, and a reachable MySQL server to manage.

```bash
cp .env.example .env
# Set ENCRYPTION_KEY in .env to a long random value before using real credentials.
docker compose up --build
```

Open [http://localhost:3000](http://localhost:3000). The API health endpoint is available at [http://localhost:8080/health](http://localhost:8080/health).

Docker Compose starts only DBfock's frontend and backend. Configure a connection to your own reachable MySQL server inside DBfock.

## Run locally in the browser

Requirements: Go 1.24+, Node.js 24+, npm, and access to a MySQL server to manage.

```bash
cp .env.example .env
cd backend && ENCRYPTION_KEY=local-development-key go run ./cmd/api

# In another terminal
cd frontend && npm install && npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Desktop app

Requirements: the local backend and frontend prerequisites. The Makefile downloads and runs the pinned Wails v2 CLI automatically.

```bash
make dev-desktop
```

`make dev-desktop` opens DBfock with live reload. It starts the local API at `127.0.0.1:8080` and stores desktop data independently from the browser development database.

Build a distributable app with:

```bash
make build-desktop
```

On macOS, the generated app is written under `backend/build/bin/` and can be opened from Finder or the terminal. The desktop app keeps its SQLite database and generated encryption key in the OS user configuration directory under `DBfock`.

## Useful commands

| Command | Description |
| --- | --- |
| `make dev-backend` | Start only the Go API for browser development. |
| `make dev-frontend` | Start only the Nuxt development server. |
| `make dev-desktop` | Start DBfock through Wails with live reload. |
| `make test` | Run the Go test suite. |
| `make typecheck` | Run the Nuxt/TypeScript checks. |
| `make build` | Build the browser API and frontend. |
| `make build-desktop` | Generate the native Wails application. |
| `make docker-up` | Build and start the web stack with Docker Compose. |

## First query

1. Choose **Create connection**, complete the MySQL form, and use **Test connection**.
2. Save it, then connect and expand the connection in the left tree.
3. Double-click a table to open its paginated data grid, or select **New SQL query**.
4. Run a query with the Run button or `Ctrl/Cmd + Enter`.
5. Save useful SQL from the editor, or create a smart query from a selected statement.

## AI setup

Open **Settings -> AI** and choose a provider:

- OpenAI
- OpenRouter
- Anthropic
- Ollama

DBfock stores AI provider settings in the local application database. API keys are encrypted with the same `ENCRYPTION_KEY` used for connection passwords. AI requests can include schema context, selected editor SQL, and selected database/table scope. Audit logs are available from **Settings -> Audit**.

## Security decisions

- Stored connection passwords and AI API keys are encrypted with AES-GCM. `ENCRYPTION_KEY` is required before storing real secrets.
- Password values are not returned in API responses, exported connection files, or logs.
- Connection records, AI settings, audit logs, and query history are scoped to the local MVP user. Do not expose DBfock to untrusted multi-user networks before real authentication/session enforcement is added.
- Object identifiers for schema navigation are allow-listed before SQL interpolation. Values used for paging are parameterized.
- Queries use context timeouts and can be cancelled by request id. Payload size, result row count, concurrent query count, and MySQL pool sizes are bounded.
- CORS is allow-listed, API requests are rate-limited, and errors are consistently shaped.

## API highlights

- `GET /health`
- `GET /api/auth/me`
- `GET|POST /api/connections`
- `GET|PUT|DELETE /api/connections/:id`
- `POST /api/connections/test`
- `GET /api/connections/export`
- `POST /api/connections/import`
- `POST /api/connections/:id/connect`
- `POST /api/connections/:id/disconnect`
- `GET /api/connections/:id/stats`
- `GET /api/connections/:id/metadata/:section`
- `GET /api/connections/:id/databases`
- `GET /api/connections/:id/databases/:database/tables`
- `GET /api/connections/:id/databases/:database/views`
- `GET /api/connections/:id/databases/:database/tables/:table/structure`
- `GET /api/connections/:id/databases/:database/tables/:table/data`
- `POST /api/connections/:id/query`
- `POST /api/connections/:id/query/cancel`
- `GET /api/connections/:id/transaction`
- `POST /api/connections/:id/transaction/commit`
- `POST /api/connections/:id/transaction/rollback`
- `GET /api/query-history`
- `DELETE /api/query-history/:id`
- `GET|PUT /api/ai/settings`
- `POST /api/ai/models`
- `POST /api/ai/chat`
- `POST /api/ai/chat/jobs`
- `GET /api/ai/chat/jobs/:id`
- `POST /api/ai/smart-queries`
- `GET /api/ai/audit-logs`

## Current limitations

DBfock is MySQL-only today. Browser development creates a local MVP user instead of enforcing full authentication. Inline record editing, DDL builders, richer object-type navigation, persisted server-side saved query storage, and additional database providers are planned extensions. Multi-statement SQL should be used one statement at a time in this release.
