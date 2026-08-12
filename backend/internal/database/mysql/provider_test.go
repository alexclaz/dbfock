package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
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

func TestProductionMutationIsOnlyQueuedUntilCommit(t *testing.T) {
	p := New(1)
	connection := models.Connection{ID: "production"}

	result, err := p.QueryInTransaction(context.Background(), connection, "ALTER TABLE users ADD COLUMN active TINYINT", 100, true)
	if err != nil {
		t.Fatalf("QueryInTransaction() error = %v", err)
	}
	if !result.TransactionPending || result.PendingStatements != 1 {
		t.Fatalf("result = %#v, want one pending statement", result)
	}
	status := p.TransactionStatus(connection)
	if !status.Pending || len(status.Statements) != 1 || status.Statements[0].SQL != "ALTER TABLE users ADD COLUMN active TINYINT" {
		t.Fatalf("status = %#v, want queued ALTER TABLE", status)
	}
	if _, err := p.RollbackTransaction(context.Background(), connection, nil); err != nil {
		t.Fatalf("RollbackTransaction() error = %v", err)
	}
	if status = p.TransactionStatus(connection); status.Pending {
		t.Fatalf("status after rollback = %#v, want no pending statements", status)
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

func TestUpdateRowStatementNormalizesRFC3339Values(t *testing.T) {
	original := map[string]any{"id": 1, "criado_em": "2026-08-04T23:14:46Z", "expira_em": "2026-08-05T02:44:46.5Z", "cancelado_em": nil, "moeda": "BRL"}
	_, args, err := updateRowStatement("geral", "creditos_recargas", original, map[string]any{"pago_em": "2026-08-05T10:00:00Z"})
	if err != nil {
		t.Fatalf("updateRowStatement() error = %v", err)
	}
	// args: [pago_em] + WHERE columns sorted: cancelado_em, criado_em, expira_em, id, moeda
	want := []any{"2026-08-05 10:00:00", nil, "2026-08-04 23:14:46", "2026-08-05 02:44:46.5", 1, "BRL"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %#v, want %#v", i, args[i], want[i])
		}
	}
}

func TestNormalizeTemporalValueLeavesNonTimestampsAlone(t *testing.T) {
	for _, value := range []any{"BRL", "2026-08-04", nil, 45, "not a date at all"} {
		if got := normalizeTemporalValue(value); got != value {
			t.Fatalf("normalizeTemporalValue(%#v) = %#v, want unchanged", value, got)
		}
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

func TestUserManagementStatementsValidateAccountAndPrivileges(t *testing.T) {
	account, err := mysqlAccount("app_user", "%")
	if err != nil || account != "'app_user'@'%'" {
		t.Fatalf("mysqlAccount() = %q, %v", account, err)
	}
	if _, err := mysqlAccount("user'; DROP USER root", "%"); err == nil {
		t.Fatal("mysqlAccount() accepted an unsafe username")
	}
	statements, err := grantStatements(account, models.DatabaseUserInput{Databases: []string{"app", "audit"}, Privileges: []string{"select", "UPDATE", "SELECT"}})
	if err != nil {
		t.Fatalf("grantStatements() error = %v", err)
	}
	want := []string{"GRANT SELECT, UPDATE ON `app`.* TO 'app_user'@'%'", "GRANT SELECT, UPDATE ON `audit`.* TO 'app_user'@'%'"}
	if !reflect.DeepEqual(statements, want) {
		t.Fatalf("grantStatements() = %q, want %q", statements, want)
	}
	statements, err = grantStatements(account, models.DatabaseUserInput{Privileges: []string{"ALL PRIVILEGES"}})
	if err != nil || !reflect.DeepEqual(statements, []string{"GRANT ALL PRIVILEGES ON *.* TO 'app_user'@'%'"}) {
		t.Fatalf("grantStatements() all privileges = %q, %v", statements, err)
	}
	if _, err := grantStatements(account, models.DatabaseUserInput{Privileges: []string{"SUPER"}}); err == nil {
		t.Fatal("grantStatements() accepted an unsupported privilege")
	}
}
