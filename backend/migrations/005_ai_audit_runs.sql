ALTER TABLE ai_audit_logs ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_audit_logs ADD COLUMN question TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ai_audit_logs_user_run_created ON ai_audit_logs(user_id, run_id, created_at DESC);
