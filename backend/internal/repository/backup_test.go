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
CREATE TABLE backup_settings (user_id TEXT PRIMARY KEY, endpoint TEXT NOT NULL);`)}}
	if err := repo.MigrateFS(ctx, migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO connections (id,user_id,name,password_encrypted) VALUES ('connection-1','local-user','Original','encrypted')`); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	script, err := repo.DumpSQL(ctx)
	if err != nil {
		t.Fatalf("dump SQL: %v", err)
	}
	if !strings.Contains(script, `INSERT INTO "connections"`) || strings.Contains(script, `"backup_settings"`) {
		t.Fatalf("unexpected dump contents:\n%s", script)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE connections SET name='Changed' WHERE id='connection-1'`); err != nil {
		t.Fatalf("change connection: %v", err)
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
}
