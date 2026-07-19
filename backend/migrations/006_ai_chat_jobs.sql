CREATE TABLE IF NOT EXISTS ai_chat_jobs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_chat_jobs_user_updated ON ai_chat_jobs(user_id, updated_at DESC);
