package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBeginTransactionSurvivesRequestCancellation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users (id, name) VALUES (1, 'System')"); err != nil {
		t.Fatalf("creating test data error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tx, err := beginTransaction(ctx, db)
	if err != nil {
		t.Fatalf("beginTransaction() error = %v", err)
	}
	p := New(1)
	if _, err := p.runWithQueryer(ctx, "UPDATE users SET name = 'SistemX' WHERE id = 1", 10, nil, tx); err != nil {
		t.Fatalf("UPDATE error = %v", err)
	}
	cancel()
	result, err := p.runWithQueryer(context.Background(), "SELECT name FROM users WHERE id = 1", 10, nil, tx)
	if err != nil {
		t.Fatalf("SELECT after request cancellation error = %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["name"] != "SistemX" {
		t.Fatalf("SELECT rows = %#v, want updated row", result.Rows)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() after request cancellation error = %v", err)
	}
}

func TestNewQueryResultSerializesEmptyCollections(t *testing.T) {
	payload, err := json.Marshal(newQueryResult())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(payload), `"columns":null`) || strings.Contains(string(payload), `"rows":null`) {
		t.Fatalf("newQueryResult() serialized null collections: %s", payload)
	}
}

func TestUpdateRowStatementUsesParametersAndNullSafeOriginalValues(t *testing.T) {
	statement, args, err := updateRowStatement("app", "users", map[string]any{"id": 7, "nickname": nil}, map[string]any{"name": "Ana"})
	if err != nil {
		t.Fatalf("updateRowStatement() error = %v", err)
	}
	want := "UPDATE `app`.`users` SET `name`=? WHERE `id` <=> ? AND `nickname` <=> ? LIMIT 1"
	if statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
	if len(args) != 3 || args[0] != "Ana" || args[1] != 7 || args[2] != nil {
		t.Fatalf("args = %#v, want parameterized changed and original values", args)
	}
}

func TestLimitSelectRows(t *testing.T) {
	got := limitSelectRows(" SELECT id FROM users; ", 200)
	want := "SELECT * FROM (SELECT id FROM users) AS `dbfock_result` LIMIT 201"
	if got != want {
		t.Fatalf("limitSelectRows() = %q, want %q", got, want)
	}
}

func TestLimitSelectRowsLeavesMutationsUnchanged(t *testing.T) {
	statement := "UPDATE users SET name = 'Ana'"
	if got := limitSelectRows(statement, 200); got != statement {
		t.Fatalf("limitSelectRows() = %q, want original statement", got)
	}
}

func TestLimitSelectRowsRespectsExplicitTopLevelLimit(t *testing.T) {
	statement := "SELECT id FROM users LIMIT 500"
	if got := limitSelectRows(statement, 200); got != statement {
		t.Fatalf("limitSelectRows() = %q, want original statement", got)
	}
	if !hasTopLevelLimit(statement) {
		t.Fatal("hasTopLevelLimit() = false, want true")
	}
}

func TestLimitSelectRowsStillCapsAnInnerLimit(t *testing.T) {
	statement := "SELECT * FROM (SELECT id FROM users LIMIT 10) AS recent"
	if hasTopLevelLimit(statement) {
		t.Fatal("hasTopLevelLimit() = true, want false")
	}
	want := "SELECT * FROM (SELECT * FROM (SELECT id FROM users LIMIT 10) AS recent) AS `dbfock_result` LIMIT 201"
	if got := limitSelectRows(statement, 200); got != want {
		t.Fatalf("limitSelectRows() = %q, want %q", got, want)
	}
}
