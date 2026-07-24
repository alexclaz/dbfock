package ai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dbfock/database-manager/backend/internal/encryption"
	"github.com/dbfock/database-manager/backend/internal/models"
	"github.com/dbfock/database-manager/backend/internal/repository"
)

type Service struct {
	repo   *repository.Repository
	cipher *encryption.Service
	client *http.Client
}

// AuditRun identifies all model calls made while answering one user question.
// It lets the audit UI keep the progressive workflow together.
type AuditRun struct {
	ID       string
	Question string
}

func NewAuditRun(question string) (AuditRun, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return AuditRun{}, fmt.Errorf("create AI audit run: %w", err)
	}
	return AuditRun{ID: hex.EncodeToString(b), Question: question}, nil
}

func New(r *repository.Repository, c *encryption.Service) *Service {
	return &Service{repo: r, cipher: c, client: &http.Client{Timeout: 45 * time.Second}}
}

func defaults(provider string) (string, string) {
	switch provider {
	case "openai":
		return "gpt-5.4", "https://api.openai.com/v1"
	case "anthropic":
		return "claude-sonnet-4-5", "https://api.anthropic.com/v1"
	case "openrouter":
		return "openai/gpt-5-mini", "https://openrouter.ai/api/v1"
	default:
		return "llama3.2", "http://localhost:11434"
	}
}

func validProvider(provider string) bool {
	return provider == "openai" || provider == "anthropic" || provider == "openrouter" || provider == "ollama"
}

func (s *Service) Save(ctx context.Context, provider, model, base, key string) (models.AISetting, error) {
	if !validProvider(provider) {
		return models.AISetting{}, fmt.Errorf("unsupported AI provider")
	}
	dm, db := defaults(provider)
	if model == "" {
		model = dm
	}
	if base == "" {
		base = db
	}
	old, _ := s.repo.GetAISetting(ctx, repository.LocalUserID)
	enc := ""
	if old.Provider == provider {
		enc = old.APIKeyEncrypted
	}
	if key != "" {
		var err error
		enc, err = s.cipher.Encrypt(key)
		if err != nil {
			return models.AISetting{}, err
		}
	}
	if provider != "ollama" && enc == "" {
		return models.AISetting{}, fmt.Errorf("an API key is required")
	}
	out := models.AISetting{Provider: provider, Model: model, BaseURL: strings.TrimRight(base, "/"), APIKeyEncrypted: enc}
	return out, s.repo.SaveAISetting(ctx, out)
}

func (s *Service) Get(ctx context.Context) (models.AISetting, error) {
	return s.repo.GetAISetting(ctx, repository.LocalUserID)
}

// ListModels validates credentials against the selected provider without persisting the API key.
func (s *Service) ListModels(ctx context.Context, provider, baseURL, apiKey string) ([]string, error) {
	if !validProvider(provider) {
		return nil, fmt.Errorf("unsupported AI provider")
	}
	if provider != "ollama" && strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("an API key is required to list models")
	}
	_, defaultBaseURL := defaults(provider)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	path := "/models"
	if provider == "ollama" {
		path = "/api/tags"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if provider == "anthropic" {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("could not list models: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var result []string
	if provider == "ollama" {
		var payload struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid models response: %w", err)
		}
		for _, model := range payload.Models {
			if model.Name != "" {
				result = append(result, model.Name)
			}
		}
	} else {
		var payload struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid models response: %w", err)
		}
		for _, model := range payload.Data {
			if model.ID != "" {
				result = append(result, model.ID)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

const chatSystemPrompt = `You are DBfock AI, an expert MySQL database assistant. Help users understand and solve SQL and database problems; you are not limited to generating queries.
You can explain SQL and database behavior, create queries, review and improve existing SQL, diagnose errors, discuss joins, filters, aggregations, transactions, execution plans, indexes, and data modeling. Answer general SQL questions directly. When the supplied schema or SQL is relevant, use it carefully; distinguish confirmed facts from assumptions and ask a concise clarifying question when needed.
The supplied schema has an accessible-schema catalog followed by column-level details. The catalog is authoritative for connection-specific databases and tables. Table, column, and database names are literal identifiers: never translate, rename, normalize, or invent them. Preserve their exact spelling, case, accents, and underscores, and use backticks around identifiers in generated SQL. If a requested identifier is absent from the supplied schema, say so instead of substituting a similar one.
For query reviews, preserve the original intent unless the user asks to change it, explain material trade-offs, and return a revised query only when useful. Prefer safe, read-only examples; for write or destructive operations, clearly call out the impact and recommend an appropriate WHERE clause, transaction, backup, or validation. Never claim to have executed SQL. Answer in the user's language. When SQL is requested or useful, explain briefly and then place it in a fenced sql block.`

func (s *Service) Chat(ctx context.Context, setting models.AISetting, prompt string) (string, error) {
	return s.ChatWithSystem(ctx, setting, chatSystemPrompt, prompt)
}

// ChatWithSystem sends a focused task to the configured provider. The AI
// workflow uses a different system instruction for each discovery step, so it
// does not have to send the full database schema with every request.
func (s *Service) ChatWithSystem(ctx context.Context, setting models.AISetting, systemPrompt, prompt string) (string, error) {
	return s.chat(ctx, setting, systemPrompt, prompt)
}

// ChatWithAudit records the exact prompt and provider response locally. Audit
// writes are best effort so the agent still works if its local audit store is
// temporarily unavailable.
func (s *Service) ChatWithAudit(ctx context.Context, setting models.AISetting, run AuditRun, stage, systemPrompt, prompt string) (string, error) {
	response, err := s.chat(ctx, setting, systemPrompt, prompt)
	audit := models.AIAuditLog{RunID: run.ID, Question: run.Question, Stage: stage, Provider: setting.Provider, Model: setting.Model, Request: "System:\n" + systemPrompt + "\n\nUser:\n" + prompt, Response: response}
	if err != nil {
		audit.Error = err.Error()
	}
	_ = s.repo.AddAIAuditLog(context.WithoutCancel(ctx), audit)
	return response, err
}

// ChatWithAuditStream forwards generated text as it arrives while preserving
// the same local audit trail used by non-streaming calls.
func (s *Service) ChatWithAuditStream(ctx context.Context, setting models.AISetting, run AuditRun, stage, systemPrompt, prompt string, onChunk func(string)) (string, error) {
	response, err := s.chatStream(ctx, setting, systemPrompt, prompt, onChunk)
	audit := models.AIAuditLog{RunID: run.ID, Question: run.Question, Stage: stage, Provider: setting.Provider, Model: setting.Model, Request: "System:\n" + systemPrompt + "\n\nUser:\n" + prompt, Response: response}
	if err != nil {
		audit.Error = err.Error()
	}
	_ = s.repo.AddAIAuditLog(context.WithoutCancel(ctx), audit)
	return response, err
}

func (s *Service) chat(ctx context.Context, setting models.AISetting, systemPrompt, prompt string) (string, error) {
	response, err := s.chatOnce(ctx, setting, systemPrompt, prompt)
	if err != nil && retryMalformedProviderResponse(err) {
		return s.chatOnce(ctx, setting, systemPrompt, prompt)
	}
	return response, err
}

// retryMalformedProviderResponse handles transient, truncated JSON returned by
// OpenAI-compatible gateways. Retrying once is safe because these calls only
// generate text and never execute SQL.
func retryMalformedProviderResponse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected end of json input") || strings.Contains(message, "unexpected eof")
}

func (s *Service) chatOnce(ctx context.Context, setting models.AISetting, systemPrompt, prompt string) (string, error) {
	key := ""
	var err error
	if setting.APIKeyEncrypted != "" {
		key, err = s.cipher.Decrypt(setting.APIKeyEncrypted)
		if err != nil {
			return "", err
		}
	}
	url := setting.BaseURL + "/chat/completions"
	body := map[string]any{"model": setting.Model, "messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": prompt}}, "temperature": 0.2}
	if setting.Provider == "ollama" {
		url = setting.BaseURL + "/api/chat"
		body = map[string]any{"model": setting.Model, "messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": prompt}}, "stream": false}
	} else if setting.Provider == "anthropic" {
		url = setting.BaseURL + "/messages"
		body = map[string]any{"model": setting.Model, "max_tokens": 2048, "system": systemPrompt, "messages": []map[string]string{{"role": "user", "content": prompt}}}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if setting.Provider == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI provider returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	if setting.Provider == "anthropic" {
		var out struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err = json.Unmarshal(b, &out); err != nil {
			return "", err
		}
		var text strings.Builder
		for _, content := range out.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
		if text.Len() == 0 {
			return "", fmt.Errorf("AI provider returned no message")
		}
		return text.String(), nil
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if setting.Provider == "ollama" {
		return out.Message.Content, nil
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("AI provider returned no message")
	}
	return out.Choices[0].Message.Content, nil
}

func (s *Service) chatStream(ctx context.Context, setting models.AISetting, systemPrompt, prompt string, onChunk func(string)) (string, error) {
	key := ""
	var err error
	if setting.APIKeyEncrypted != "" {
		key, err = s.cipher.Decrypt(setting.APIKeyEncrypted)
		if err != nil {
			return "", err
		}
	}
	url := setting.BaseURL + "/chat/completions"
	body := map[string]any{"model": setting.Model, "messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": prompt}}, "temperature": 0.2, "stream": true}
	if setting.Provider == "ollama" {
		url = setting.BaseURL + "/api/chat"
		body = map[string]any{"model": setting.Model, "messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": prompt}}, "stream": true}
	} else if setting.Provider == "anthropic" {
		url = setting.BaseURL + "/messages"
		body = map[string]any{"model": setting.Model, "max_tokens": 2048, "system": systemPrompt, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": true}
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if setting.Provider == "anthropic" {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return "", fmt.Errorf("AI provider returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var text strings.Builder
	appendChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		text.WriteString(chunk)
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2<<20))
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "[DONE]" {
			break
		}
		var chunk string
		if setting.Provider == "anthropic" {
			var event struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(line), &event) == nil && event.Type == "content_block_delta" {
				chunk = event.Delta.Text
			}
		} else if setting.Provider == "ollama" {
			var event struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &event) == nil {
				chunk = event.Message.Content
			}
		} else {
			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(line), &event) == nil && len(event.Choices) > 0 {
				chunk = event.Choices[0].Delta.Content
			}
		}
		appendChunk(chunk)
	}
	if err := scanner.Err(); err != nil {
		return text.String(), err
	}
	if text.Len() == 0 {
		return "", fmt.Errorf("AI provider returned no streamed message")
	}
	return text.String(), nil
}
