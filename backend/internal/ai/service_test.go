package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestChatRetriesTruncatedProviderJSON(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"message":`))
			return
		}
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	response, err := service.Chat(context.Background(), models.AISetting{Provider: "ollama", Model: "test", BaseURL: server.URL}, "hello")
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response != "ok" {
		t.Fatalf("Chat() response = %q, want ok", response)
	}
	if requests.Load() != 2 {
		t.Fatalf("provider requests = %d, want 2", requests.Load())
	}
}
