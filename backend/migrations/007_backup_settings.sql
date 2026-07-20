CREATE TABLE IF NOT EXISTS backup_settings (
  user_id TEXT PRIMARY KEY,
  endpoint TEXT NOT NULL,
  bucket TEXT NOT NULL,
  region TEXT NOT NULL,
  access_key_encrypted TEXT NOT NULL DEFAULT '',
  secret_encrypted TEXT NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);
