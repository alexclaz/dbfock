package httpapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/config"
	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestGetAppVersion(t *testing.T) {
	api := &API{config: config.Config{AppVersion: "0.5.8"}}
	recorder := httptest.NewRecorder()
	api.getAppVersion(recorder, httptest.NewRequest("GET", "/api/app/version", nil))

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Version != "0.5.8" {
		t.Fatalf("version = %q, want 0.5.8", response.Version)
	}
}

func TestProductionMutationClassificationProtectsSchemaAndDataChanges(t *testing.T) {
	for _, operation := range []string{"CREATE", "ALTER", "DROP", "TRUNCATE", "INSERT", "UPDATE", "DELETE", "GRANT"} {
		if !isProductionMutation(operation) {
			t.Errorf("isProductionMutation(%q) = false, want true", operation)
		}
	}
	for _, operation := range []string{"SELECT", "SHOW", "DESCRIBE", "EXPLAIN", "USE", "SET", "DO"} {
		if isProductionMutation(operation) {
			t.Errorf("isProductionMutation(%q) = true, want false", operation)
		}
	}
}

func TestConnectionExportRequestOmitsPassword(t *testing.T) {
	connection := models.Connection{
		Name:              "Production",
		Driver:            "mysql",
		Host:              "db.example.com",
		Port:              3306,
		Username:          "dbfock",
		PasswordEncrypted: "secret-that-must-not-be-exported",
		InitialDatabase:   "app",
		Color:             "#3B82F6",
		Environment:       "production",
		SSLEnabled:        true,
		TimeoutSeconds:    45,
	}

	exported := connectionExportRequest(connection)
	if exported.Password != "" {
		t.Fatal("connection export must not include the password")
	}
	payload, err := json.Marshal(connectionExport{Version: 1, Connections: []connectionRequest{exported}})
	if err != nil {
		t.Fatalf("marshal connection export: %v", err)
	}
	if strings.Contains(string(payload), "password") || strings.Contains(string(payload), connection.PasswordEncrypted) {
		t.Fatalf("connection export leaked a password: %s", payload)
	}
	if exported.Name != connection.Name || exported.Host != connection.Host || exported.Port != connection.Port || !exported.SSLEnabled {
		t.Fatalf("connection configuration was not preserved: %#v", exported)
	}
}

func TestRequireConnected(t *testing.T) {
	api := &API{sessions: map[string]bool{"connected": true}}

	if err := api.requireConnected("connected"); err != nil {
		t.Fatalf("connected database was rejected: %v", err)
	}
	if err := api.requireConnected("disconnected"); !errors.Is(err, errDatabaseNotConnected) {
		t.Fatalf("disconnected database error = %v, want %v", err, errDatabaseNotConnected)
	}
}

func TestMigrationHistoryKeepsWhichTablesFailedAndWhy(t *testing.T) {
	summary := migrationFailureSummary([]models.DatabaseMigrationFailure{
		{Database: "shop", Table: "orders", Error: "Error 1153: Got a packet bigger than 'max_allowed_packet' bytes"},
		{Database: "shop", Table: "invoices", Error: "Error 1292: Incorrect datetime value"},
	})
	if !strings.Contains(summary, "2 table(s) could not be migrated") {
		t.Fatalf("summary lost the failure count: %s", summary)
	}
	for _, expected := range []string{"shop.orders", "max_allowed_packet", "shop.invoices", "Incorrect datetime value"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary lost %q: %s", expected, summary)
		}
	}
}

func TestMigrationHistoryStaysBoundedWhenEveryTableFails(t *testing.T) {
	failures := make([]models.DatabaseMigrationFailure, 500)
	for index := range failures {
		failures[index] = models.DatabaseMigrationFailure{Database: "shop", Table: "orders", Error: strings.Repeat("x", 2_000)}
	}
	summary := migrationFailureSummary(failures)
	if len(summary) > migrationFailureSummaryLimit+200 {
		t.Fatalf("summary grew to %d characters", len(summary))
	}
	if !strings.Contains(summary, "more") {
		t.Fatalf("truncated summary does not say how many failures were dropped: %s", summary)
	}
}
