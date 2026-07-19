CREATE TABLE IF NOT EXISTS ai_settings (
  user_id TEXT PRIMARY KEY, provider TEXT NOT NULL, model TEXT NOT NULL, base_url TEXT NOT NULL,
  api_key_encrypted TEXT NOT NULL DEFAULT '', updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);
