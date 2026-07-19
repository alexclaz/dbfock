CREATE TABLE IF NOT EXISTS ai_audit_logs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  stage TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  request_text TEXT NOT NULL,
  response_text TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_ai_audit_logs_user_created ON ai_audit_logs(user_id, created_at DESC);
