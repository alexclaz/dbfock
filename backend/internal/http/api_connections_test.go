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
	api := &API{config: config.Config{AppVersion: "0.5.5"}}
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
	if response.Version != "0.5.5" {
		t.Fatalf("version = %q, want 0.5.5", response.Version)
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
