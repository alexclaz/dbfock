# DBfock

DBfock is a MySQL workspace inspired by database IDEs such as DBeaver and DataGrip, with a deliberately quieter, minimal interface. It runs in the browser or as a native desktop app built with Wails. This first release implements the complete core path: create and test a connection, securely save it, browse databases and tables, inspect data/DDL, run SQL, and reuse local query history.

## Architecture

```text
Nuxt 4 / Vue 3 / TypeScript
        │ REST
Go + Chi API ── provider registry ── MySQL (`database/sql`)
        │
SQLite application store (connections, users, query history)

Wails v2 packages the Nuxt build and starts the local Go API in-process.
```

- **Backend:** Go 1.24, Chi, `database/sql`, `github.com/go-sql-driver/mysql`, and SQLite through `modernc.org/sqlite`.
- **Frontend:** Nuxt 4.4, Vue 3 Composition API, Pinia, and Tailwind CSS.
- **Desktop:** Wails v2 packages the Nuxt static build with the Go application; the local API and encrypted SQLite store run in the same process.
- **Provider boundary:** the `database.Provider` interface isolates database-specific discovery, data, and query behavior. A PostgreSQL, SQL Server, or SQLite provider can be added without changing HTTP consumers.
- **Application data vs. managed data:** SQLite only persists DBfock metadata. It never replaces or mingles with the MySQL instances being managed.

## Directory structure

```text
backend/
  main.go                  Wails desktop entrypoint
  wails.json               Wails build configuration
  cmd/api/                 API entrypoint
  desktop/assets/          Generated Nuxt assets embedded by Wails
  internal/{config,connections,database,encryption,http,middleware,models,repository}/
  migrations/              SQLite application migrations
frontend/
  app/
    components/ pages/ stores/ types/ composables/ assets/ layouts/
docker-compose.yml
```

## Run with Docker

```bash
cp .env.example .env
# Set ENCRYPTION_KEY in .env to a long random value.
docker compose up --build
```

Open [http://localhost:3000](http://localhost:3000). The API health endpoint is available at [http://localhost:8080/health](http://localhost:8080/health).

Docker Compose starts only the DBfock frontend and backend. Configure a connection to your own reachable MySQL server in DBfock.

## Run in the browser

Requirements: Go 1.24+, Node.js 24+, npm, and access to a MySQL server to manage.

```bash
cp .env.example .env
cd backend && ENCRYPTION_KEY=local-development-key go run ./cmd/api
# In another terminal
cd frontend && npm install && npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Desktop app (Wails)

Requirements: the local prerequisites for the backend and frontend. The Makefile downloads and runs the pinned Wails v2 CLI automatically on first use.

```bash
make dev-desktop
```

`make dev-desktop` opens DBfock with live reload. It starts the local API at `127.0.0.1:8080` and stores desktop data independently from the browser development database.

Build a distributable app with:

```bash
# From the repository root
make build-desktop
ditto -c -k --sequesterRsrc --keepParent backend/build/bin/DBFock.app DBFock-macOS.zip
# Or, from backend/
make build-desktop
```

On macOS, the generated app is `backend/build/bin/dbfock.app` and can be opened with:

```bash
open backend/build/bin/dbfock.app
```

The desktop app keeps its SQLite database and a generated encryption key in the OS user configuration directory under `DBfock`.

## Useful commands

| Command | Description |
| --- | --- |
| `make dev-backend` | Start only the API for browser development. |
| `make dev-frontend` | Start only the Nuxt development server. |
| `make dev-desktop` | Start DBfock through Wails with live reload. |
| `make test` | Run the Go test suite. |
| `make typecheck` | Run the Nuxt/TypeScript checks. |
| `make build-desktop` | Generate the native Wails application. |
| `make docker-up` | Build and start the web stack with Docker Compose. |

## First query

1. Choose **Create connection**, complete the MySQL form, and use **Test connection**.
2. Save it, then expand the connection in the left tree.
3. Double-click a table to open its paginated data grid or select **New SQL query**.
4. Run a query against one of your tables with the Run button or `Ctrl/Cmd + Enter`.

## Security decisions

- Stored connection passwords are encrypted using AES-GCM; `ENCRYPTION_KEY` is mandatory and no password is returned in any API response.
- Connection records and query history are already scoped to `user_id`. The MVP creates a local user while authentication is being introduced; do not expose it to untrusted multi-user networks before real authentication/session enforcement is added.
- Object identifiers for schema navigation are allow-listed before SQL interpolation. Values used for paging are parameterized.
- Queries use context timeouts and can be cancelled by request id. Payloads and result row counts are bounded. MySQL pools have capped open and idle connections.
- CORS is allow-listed, API requests are rate-limited, errors are consistently shaped, and password values are never logged.

## API highlights

`POST /api/connections/test`, CRUD `/api/connections`, metadata/data endpoints under `/api/connections/:id/databases/...`, `POST /api/connections/:id/query`, `POST /api/connections/:id/query/cancel`, and `GET /api/query-history` are implemented. `GET /api/auth/me` exposes the current local MVP identity.

## Current limitations and next steps

This MVP is MySQL-only; tree nodes currently cover databases, base tables, and table details. Inline record editing, DDL builders, saved scripts, drag-reordering tabs, Monaco/CodeMirror autocomplete, persistent sessions/JWT authentication, additional object types, and PostgreSQL/SQL Server/SQLite providers are planned extensions. Multi-statement SQL should be used one statement at a time in this release.
