CREATE TABLE IF NOT EXISTS saved_queries (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  name TEXT NOT NULL,
  sql_text TEXT NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(connection_id) REFERENCES connections(id)
);
CREATE INDEX IF NOT EXISTS idx_saved_queries_user_updated ON saved_queries(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS smart_queries (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  connection_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  sql_text TEXT NOT NULL,
  source_sql TEXT NOT NULL DEFAULT '',
  parameters_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(connection_id) REFERENCES connections(id)
);
CREATE INDEX IF NOT EXISTS idx_smart_queries_user_created ON smart_queries(user_id, created_at DESC);
