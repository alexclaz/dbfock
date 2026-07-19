package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAISettingPublicMarksSavedConfiguration(t *testing.T) {
	setting := AISetting{Provider: "openai", Model: "gpt-5.4", BaseURL: "https://api.openai.com/v1", APIKeyEncrypted: "encrypted"}

	got := setting.Public()
	if !got.Configured {
		t.Fatal("saved AI setting must be marked as configured")
	}
	if !got.HasAPIKey {
		t.Fatal("saved API key must be reported without exposing its value")
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	for _, field := range []string{`"provider":"openai"`, `"model":"gpt-5.4"`, `"baseUrl":"https://api.openai.com/v1"`} {
		if !strings.Contains(string(payload), field) {
			t.Errorf("response is missing %s: %s", field, payload)
		}
	}
}
