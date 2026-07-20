package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDumpAndRestoreSQLRoundTripWorkspaceData(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repo.Close()
	migrations := fstest.MapFS{"001.sql": {Data: []byte(`CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL);
CREATE TABLE connections (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, password_encrypted TEXT NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id));
CREATE TABLE backup_settings (user_id TEXT PRIMARY KEY, endpoint TEXT NOT NULL);
CREATE TABLE saved_queries (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, connection_id TEXT NOT NULL, name TEXT NOT NULL, sql_text TEXT NOT NULL, updated_at DATETIME NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id), FOREIGN KEY(connection_id) REFERENCES connections(id));
CREATE TABLE smart_queries (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, connection_id TEXT NOT NULL, title TEXT NOT NULL, description TEXT NOT NULL, sql_text TEXT NOT NULL, source_sql TEXT NOT NULL, parameters_json TEXT NOT NULL, created_at DATETIME NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id), FOREIGN KEY(connection_id) REFERENCES connections(id));`)}}
	if err := repo.MigrateFS(ctx, migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO connections (id,user_id,name,password_encrypted) VALUES ('connection-1','local-user','Original','encrypted')`); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO saved_queries (id,user_id,connection_id,name,sql_text,updated_at) VALUES ('saved-1','local-user','connection-1','Saved','SELECT 1','2026-01-01')`); err != nil {
		t.Fatalf("seed saved query: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO smart_queries (id,user_id,connection_id,title,description,sql_text,source_sql,parameters_json,created_at) VALUES ('smart-1','local-user','connection-1','Smart','Description','SELECT :id','SELECT :id','[{"key":"id","defaultValue":"1"}]','2026-01-01')`); err != nil {
		t.Fatalf("seed smart query: %v", err)
	}
	script, err := repo.DumpSQL(ctx)
	if err != nil {
		t.Fatalf("dump SQL: %v", err)
	}
	if !strings.Contains(script, `INSERT INTO "connections"`) || !strings.Contains(script, `INSERT INTO "saved_queries"`) || !strings.Contains(script, `INSERT INTO "smart_queries"`) || strings.Contains(script, `"backup_settings"`) {
		t.Fatalf("unexpected dump contents:\n%s", script)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE connections SET name='Changed' WHERE id='connection-1'`); err != nil {
		t.Fatalf("change connection: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM saved_queries; DELETE FROM smart_queries`); err != nil {
		t.Fatalf("change workspace queries: %v", err)
	}
	if err := repo.RestoreSQL(ctx, script); err != nil {
		t.Fatalf("restore SQL: %v", err)
	}
	var name string
	if err := repo.db.QueryRowContext(ctx, `SELECT name FROM connections WHERE id='connection-1'`).Scan(&name); err != nil {
		t.Fatalf("read restored connection: %v", err)
	}
	if name != "Original" {
		t.Fatalf("connection name = %q, want Original", name)
	}
	var saved, smart int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM saved_queries`).Scan(&saved); err != nil {
		t.Fatalf("read restored saved queries: %v", err)
	}
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM smart_queries`).Scan(&smart); err != nil {
		t.Fatalf("read restored smart queries: %v", err)
	}
	if saved != 1 || smart != 1 {
		t.Fatalf("restored workspace queries = saved:%d smart:%d, want 1 each", saved, smart)
	}
}
