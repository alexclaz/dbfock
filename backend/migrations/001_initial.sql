CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
  created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS connections (
  id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, driver TEXT NOT NULL,
  host TEXT NOT NULL, port INTEGER NOT NULL, username TEXT NOT NULL, password_encrypted TEXT NOT NULL,
  initial_database TEXT NOT NULL DEFAULT '', ssl_enabled BOOLEAN NOT NULL DEFAULT FALSE, timeout_seconds INTEGER NOT NULL DEFAULT 30,
  created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_connections_user_id ON connections(user_id);
CREATE TABLE IF NOT EXISTS query_history (
  id TEXT PRIMARY KEY, user_id TEXT NOT NULL, connection_id TEXT NOT NULL, sql_text TEXT NOT NULL,
  operation_type TEXT NOT NULL, status TEXT NOT NULL, error_message TEXT NOT NULL DEFAULT '', execution_time_ms INTEGER NOT NULL DEFAULT 0,
  affected_rows INTEGER NOT NULL DEFAULT 0, created_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id), FOREIGN KEY(connection_id) REFERENCES connections(id)
);
CREATE INDEX IF NOT EXISTS idx_query_history_user_created ON query_history(user_id, created_at DESC);
