package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestChatSystemPromptDefinesASQLAssistant(t *testing.T) {
	for _, expected := range []string{"not limited to generating queries", "explain SQL", "review and improve", "diagnose errors", "indexes, and data modeling"} {
		if !strings.Contains(chatSystemPrompt, expected) {
			t.Errorf("chat system prompt is missing %q", expected)
		}
	}
}

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

func TestChatWithAuditStreamForOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/chat") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"ola \"}}\n{\"message\":{\"content\":\"mundo\"}}\n"))
	}))
	defer server.Close()

	service := &Service{client: server.Client()}
	chunks := []string{}
	response, err := service.chatStream(context.Background(), models.AISetting{Provider: "ollama", Model: "test", BaseURL: server.URL}, "system", "hello", func(chunk string) { chunks = append(chunks, chunk) })
	if err != nil {
		t.Fatalf("chatStream() error = %v", err)
	}
	if response != "ola mundo" || strings.Join(chunks, "") != response {
		t.Fatalf("response/chunks = %q/%q, want ola mundo", response, strings.Join(chunks, ""))
	}
}
