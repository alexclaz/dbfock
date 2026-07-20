package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dbfock/database-manager/backend/internal/ai"
	"github.com/dbfock/database-manager/backend/internal/backup"
	"github.com/dbfock/database-manager/backend/internal/config"
	"github.com/dbfock/database-manager/backend/internal/connections"
	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/models"
	"github.com/dbfock/database-manager/backend/internal/repository"
	"github.com/go-chi/chi/v5"
)

type API struct {
	config      config.Config
	connections *connections.Service
	providers   *database.Registry
	repo        *repository.Repository
	log         *slog.Logger
	sessions    map[string]bool
	sessionMu   sync.RWMutex
	cancels     map[string]context.CancelFunc
	cancelMu    sync.Mutex
	querySlots  chan struct{}
	ai          *ai.Service
	backup      *backup.Service
}

var errDatabaseNotConnected = errors.New("connect to the database before running queries")

func New(cfg config.Config, cs *connections.Service, providers *database.Registry, repo *repository.Repository, aiService *ai.Service, backupService *backup.Service, logger *slog.Logger) *API {
	return &API{config: cfg, connections: cs, providers: providers, repo: repo, ai: aiService, backup: backupService, log: logger, sessions: map[string]bool{}, cancels: map[string]context.CancelFunc{}, querySlots: make(chan struct{}, cfg.MaxConcurrentQueries)}
}
func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(a.requestLogger)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api", func(r chi.Router) {
		r.Get("/ai/settings", a.getAISettings)
		r.Put("/ai/settings", a.saveAISettings)
		r.Post("/ai/models", a.listAIModels)
		r.Get("/ai/audit-logs", a.listAIAuditLogs)
		r.Get("/backup/settings", a.getBackupSettings)
		r.Put("/backup/settings", a.saveBackupSettings)
		r.Post("/backup/create", a.createBackup)
		r.Post("/backup/restore", a.restoreBackup)
		r.Post("/ai/chat", a.aiChat)
		r.Post("/ai/smart-queries", a.createSmartQuery)
		r.Post("/ai/chat/jobs", a.createAIChatJob)
		r.Get("/ai/chat/jobs/{id}", a.getAIChatJob)
		r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
			respond(w, http.StatusOK, map[string]any{"id": repository.LocalUserID, "name": "Local user", "email": "local@dbfock.local"})
		})
		r.Route("/connections", func(r chi.Router) {
			r.Get("/", a.listConnections)
			r.Post("/", a.createConnection)
			r.Get("/export", a.exportConnections)
			r.Post("/import", a.importConnections)
			r.Post("/test", a.testConnection)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", a.getConnection)
				r.Put("/", a.updateConnection)
				r.Delete("/", a.deleteConnection)
				r.Post("/connect", a.connect)
				r.Post("/disconnect", a.disconnect)
				r.Get("/stats", a.connectionStats)
				r.Get("/metadata/{section}", a.connectionMetadata)
				r.Get("/databases", a.listDatabases)
				r.Get("/databases/{database}/tables", a.listTables)
				r.Get("/databases/{database}/views", a.listViews)
				r.Get("/databases/{database}/tables/{table}/structure", a.structure)
				r.Get("/databases/{database}/tables/{table}/data", a.tableData)
				r.Post("/query", a.query)
				r.Post("/query/cancel", a.cancelQuery)
				r.Get("/transaction", a.transactionStatus)
				r.Post("/transaction/commit", a.commitTransaction)
				r.Post("/transaction/rollback", a.rollbackTransaction)
			})
		})
		r.Get("/query-history", a.listHistory)
		r.Delete("/query-history/{id}", a.deleteHistory)
	})
	return r
}

type connectionRequest struct {
	Name            string `json:"name"`
	Driver          string `json:"driver"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password,omitempty"`
	InitialDatabase string `json:"initialDatabase"`
	Color           string `json:"color"`
	Environment     string `json:"environment"`
	SSLEnabled      bool   `json:"sslEnabled"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
}

type connectionExport struct {
	Version     int                 `json:"version"`
	Connections []connectionRequest `json:"connections"`
}

type connectionImportResult struct {
	Imported int `json:"imported"`
}

func (r connectionRequest) input() connections.Input {
	return connections.Input{Name: r.Name, Driver: defaultDriver(r.Driver), Host: r.Host, Port: r.Port, Username: r.Username, Password: r.Password, InitialDatabase: r.InitialDatabase, Color: r.Color, Environment: r.Environment, SSLEnabled: r.SSLEnabled, TimeoutSeconds: defaultTimeout(r.TimeoutSeconds)}
}
func defaultDriver(v string) string {
	if v == "" {
		return "mysql"
	}
	return v
}
func defaultTimeout(v int) int {
	if v == 0 {
		return 30
	}
	return v
}
func (a *API) listConnections(w http.ResponseWriter, r *http.Request) {
	items, err := a.connections.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]models.ConnectionResponse, 0, len(items))
	for _, c := range items {
		out = append(out, c.Public(a.status(c.ID)))
	}
	respond(w, http.StatusOK, out)
}
func connectionExportRequest(c models.Connection) connectionRequest {
	return connectionRequest{Name: c.Name, Driver: c.Driver, Host: c.Host, Port: c.Port, Username: c.Username, InitialDatabase: c.InitialDatabase, Color: c.Color, Environment: c.Environment, SSLEnabled: c.SSLEnabled, TimeoutSeconds: c.TimeoutSeconds}
}
func (a *API) exportConnections(w http.ResponseWriter, r *http.Request) {
	items, err := a.connections.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	out := connectionExport{Version: 1, Connections: make([]connectionRequest, 0, len(items))}
	for _, c := range items {
		out.Connections = append(out.Connections, connectionExportRequest(c))
	}
	respond(w, http.StatusOK, out)
}
func (a *API) importConnections(w http.ResponseWriter, r *http.Request) {
	var exported connectionExport
	if err := decode(w, r, &exported); err != nil {
		return
	}
	if exported.Version != 1 {
		fail(w, fmt.Errorf("unsupported connection export version"))
		return
	}
	if len(exported.Connections) > 1000 {
		fail(w, fmt.Errorf("a connection export can contain at most 1000 connections"))
		return
	}
	for i, item := range exported.Connections {
		if err := connections.ValidateImport(item.input()); err != nil {
			fail(w, fmt.Errorf("connection %d: %w", i+1, err))
			return
		}
	}
	for _, item := range exported.Connections {
		if _, err := a.connections.Import(r.Context(), item.input()); err != nil {
			fail(w, err)
			return
		}
	}
	respond(w, http.StatusCreated, connectionImportResult{Imported: len(exported.Connections)})
}
func (a *API) getConnection(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, c.Public(a.status(c.ID)))
}
func (a *API) createConnection(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if err := decode(w, r, &req); err != nil {
		return
	}
	c, err := a.connections.Create(r.Context(), req.input())
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusCreated, c.Public("disconnected"))
}
func (a *API) updateConnection(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if err := decode(w, r, &req); err != nil {
		return
	}
	a.rollbackPendingTransaction(r.Context(), chi.URLParam(r, "id"))
	c, err := a.connections.Update(r.Context(), chi.URLParam(r, "id"), req.input())
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, c.Public(a.status(c.ID)))
}
func (a *API) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a.rollbackPendingTransaction(r.Context(), id)
	if err := a.connections.Delete(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	a.sessionMu.Lock()
	delete(a.sessions, id)
	a.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) testConnection(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if err := decode(w, r, &req); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(defaultTimeout(req.TimeoutSeconds))*time.Second)
	defer cancel()
	if err := a.connections.Test(ctx, req.input()); err != nil {
		fail(w, fmt.Errorf("connection test failed: %w", err))
		return
	}
	respond(w, http.StatusOK, map[string]bool{"ok": true})
}
func (a *API) connect(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	if err = p.TestConnection(ctx, c); err != nil {
		fail(w, fmt.Errorf("connection failed: %w", err))
		return
	}
	a.sessionMu.Lock()
	a.sessions[c.ID] = true
	a.sessionMu.Unlock()
	respond(w, http.StatusOK, c.Public("connected"))
}
func (a *API) disconnect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a.rollbackPendingTransaction(r.Context(), id)
	a.sessionMu.Lock()
	delete(a.sessions, id)
	a.sessionMu.Unlock()
	respond(w, http.StatusOK, map[string]string{"status": "disconnected"})
}
func (a *API) connectionStats(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	stats, err := a.repo.ConnectionStats(r.Context(), repository.LocalUserID, c.ID)
	if err != nil {
		fail(w, err)
		return
	}
	stats.SavedConnectionCount, err = a.repo.ConnectionCount(r.Context(), repository.LocalUserID)
	if err != nil {
		fail(w, err)
		return
	}
	a.sessionMu.Lock()
	stats.ActiveConnectionCount = len(a.sessions)
	a.sessionMu.Unlock()
	p, err := a.providers.Get(c.Driver)
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
		defer cancel()
		databases, schemaErr := p.ListDatabases(ctx, c)
		if schemaErr == nil {
			stats.Schema.Available = true
			stats.Schema.Databases = len(databases)
			for _, databaseInfo := range databases {
				tables, tablesErr := p.ListTables(ctx, c, databaseInfo.Name, false)
				if tablesErr != nil {
					stats.Schema.Available = false
					stats.Schema.Error = tablesErr.Error()
					break
				}
				stats.Schema.Tables += len(tables)
			}
		} else {
			stats.Schema.Error = schemaErr.Error()
		}
	}
	respond(w, http.StatusOK, stats)
}
func (a *API) connectionMetadata(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := a.requireConnected(c.ID); err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
	defer cancel()
	metadata, err := p.ConnectionMetadata(ctx, c, chi.URLParam(r, "section"))
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, metadata)
}
func (a *API) metadata(w http.ResponseWriter, r *http.Request, fn func(context.Context, database.Provider, models.Connection) (any, error)) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
	defer cancel()
	data, err := fn(ctx, p, c)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, data)
}
func (a *API) listDatabases(w http.ResponseWriter, r *http.Request) {
	a.metadata(w, r, func(ctx context.Context, p database.Provider, c models.Connection) (any, error) {
		return p.ListDatabases(ctx, c)
	})
}
func (a *API) listTables(w http.ResponseWriter, r *http.Request) {
	db := chi.URLParam(r, "database")
	a.metadata(w, r, func(ctx context.Context, p database.Provider, c models.Connection) (any, error) {
		return p.ListTables(ctx, c, db, false)
	})
}
func (a *API) listViews(w http.ResponseWriter, r *http.Request) {
	db := chi.URLParam(r, "database")
	a.metadata(w, r, func(ctx context.Context, p database.Provider, c models.Connection) (any, error) {
		return p.ListTables(ctx, c, db, true)
	})
}
func (a *API) structure(w http.ResponseWriter, r *http.Request) {
	db, table := chi.URLParam(r, "database"), chi.URLParam(r, "table")
	a.metadata(w, r, func(ctx context.Context, p database.Provider, c models.Connection) (any, error) {
		return p.GetTableStructure(ctx, c, db, table)
	})
}
func (a *API) tableData(w http.ResponseWriter, r *http.Request) {
	db, table := chi.URLParam(r, "database"), chi.URLParam(r, "table")
	limit := queryInt(r, "limit", 100, 1, a.config.MaxQueryRows)
	offset := queryInt(r, "offset", 0, 0, 10000000)
	sort, dir := r.URL.Query().Get("sort"), r.URL.Query().Get("direction")
	a.metadata(w, r, func(ctx context.Context, p database.Provider, c models.Connection) (any, error) {
		return p.GetTableData(ctx, c, db, table, limit, offset, sort, dir)
	})
}

type queryRequest struct {
	SQL         string `json:"sql"`
	RequestID   string `json:"requestId"`
	HistorySQL  string `json:"historySql"`
	SkipHistory bool   `json:"skipHistory"`
}

func (a *API) query(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := decode(w, r, &req); err != nil {
		return
	}
	if sqlText := strings.TrimSpace(req.SQL); sqlText == "" {
		fail(w, fmt.Errorf("SQL is required"))
		return
	} else if len(sqlText) > 100000 {
		fail(w, fmt.Errorf("SQL exceeds the 100 KB limit"))
		return
	}
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := a.requireConnected(c.ID); err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
	defer cancel()
	select {
	case a.querySlots <- struct{}{}:
		defer func() { <-a.querySlots }()
	case <-ctx.Done():
		fail(w, fmt.Errorf("query queue timeout: %w", ctx.Err()))
		return
	}
	if req.RequestID != "" {
		a.cancelMu.Lock()
		a.cancels[req.RequestID] = cancel
		a.cancelMu.Unlock()
		defer func() { a.cancelMu.Lock(); delete(a.cancels, req.RequestID); a.cancelMu.Unlock() }()
	}
	historySQL := req.SQL
	if strings.TrimSpace(req.HistorySQL) != "" {
		historySQL = req.HistorySQL
	}
	operation := operationType(historySQL)
	if c.Environment == "production" && (operation == "COMMIT" || operation == "ROLLBACK") {
		fail(w, fmt.Errorf("use the transaction controls to commit or rollback production changes"))
		return
	}
	var result *models.QueryResult
	if transactional, ok := p.(database.TransactionalProvider); ok && c.Environment == "production" && (isDataMutation(operation) || transactional.TransactionStatus(c).Pending) {
		if transactional.TransactionStatus(c).Pending && !isDataMutation(operation) && !isReadOnly(operation) {
			fail(w, fmt.Errorf("commit or rollback pending production changes before running %s", operation))
			return
		}
		result, err = transactional.QueryInTransaction(ctx, c, req.SQL, a.config.MaxQueryRows, isDataMutation(operation))
	} else {
		result, err = p.Query(ctx, c, req.SQL, a.config.MaxQueryRows)
	}
	h := models.QueryHistory{ConnectionID: c.ID, SQL: historySQL, Type: operation, Status: "success"}
	if result != nil {
		h.ExecutionTimeMs = result.ExecutionTimeMs
		h.AffectedRows = result.AffectedRows
	}
	if err != nil {
		h.Status = "error"
		h.ErrorMessage = err.Error()
		if !req.SkipHistory {
			_ = a.repo.AddHistory(context.Background(), h)
		}
		fail(w, err)
		return
	}
	if !req.SkipHistory {
		if e := a.repo.AddHistory(context.Background(), h); e != nil {
			a.log.Error("could not store query history", "error", e)
		}
	}
	respond(w, http.StatusOK, result)
}
func (a *API) transactionStatus(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	if transactional, ok := p.(database.TransactionalProvider); ok {
		respond(w, http.StatusOK, transactional.TransactionStatus(c))
		return
	}
	respond(w, http.StatusOK, models.TransactionStatus{})
}
func (a *API) commitTransaction(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	transactional, ok := p.(database.TransactionalProvider)
	if !ok {
		respond(w, http.StatusOK, models.TransactionStatus{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
	defer cancel()
	status, err := transactional.CommitTransaction(ctx, c)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, status)
}
func (a *API) rollbackTransaction(w http.ResponseWriter, r *http.Request) {
	c, err := a.connection(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		fail(w, err)
		return
	}
	if transactional, ok := p.(database.TransactionalProvider); ok {
		ctx, cancel := context.WithTimeout(r.Context(), a.timeout(c))
		defer cancel()
		if err := transactional.RollbackTransaction(ctx, c); err != nil {
			fail(w, err)
			return
		}
	}
	respond(w, http.StatusOK, models.TransactionStatus{})
}
func (a *API) cancelQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := decode(w, r, &req); err != nil {
		return
	}
	a.cancelMu.Lock()
	cancel, ok := a.cancels[req.RequestID]
	a.cancelMu.Unlock()
	if ok {
		cancel()
		respond(w, http.StatusOK, map[string]bool{"cancelled": true})
		return
	}
	respond(w, http.StatusOK, map[string]bool{"cancelled": false})
}
func (a *API) listHistory(w http.ResponseWriter, r *http.Request) {
	items, err := a.repo.ListHistory(r.Context(), repository.LocalUserID, queryInt(r, "limit", 50, 1, 200))
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, items)
}

type aiSettingsRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
}

func (a *API) getAISettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.ai.Get(r.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, s.Public())
}
func (a *API) saveAISettings(w http.ResponseWriter, r *http.Request) {
	var in aiSettingsRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	s, err := a.ai.Save(r.Context(), in.Provider, in.Model, in.BaseURL, in.APIKey)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, s.Public())
}

func (a *API) listAIAuditLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := a.repo.ListAIAuditLogs(r.Context(), repository.LocalUserID, queryInt(r, "limit", 100, 1, 500))
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, logs)
}

type backupSettingsRequest struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	Secret    string `json:"secret"`
}

func (a *API) getBackupSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.backup.Get(r.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respond(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, s.Public())
}
func (a *API) saveBackupSettings(w http.ResponseWriter, r *http.Request) {
	var in backupSettingsRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	s, err := a.backup.Save(r.Context(), in.Endpoint, in.Bucket, in.Region, in.AccessKey, in.Secret)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, s.Public())
}
func (a *API) createBackup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := a.backup.Create(ctx); err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"backedUp": true})
}
func (a *API) restoreBackup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := a.backup.Restore(ctx); err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"restored": true})
}
func (a *API) listAIModels(w http.ResponseWriter, r *http.Request) {
	var in aiSettingsRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	models, err := a.ai.ListModels(ctx, in.Provider, in.BaseURL, in.APIKey)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, map[string][]string{"models": models})
}

type aiChatRequest struct {
	ConnectionID      string          `json:"connectionId"`
	Database          string          `json:"database"`
	Prompt            string          `json:"prompt"`
	History           []aiChatMessage `json:"history"`
	DatabaseScope     string          `json:"databaseScope"`
	SelectedDatabases []string        `json:"selectedDatabases"`
	TableScope        string          `json:"tableScope"`
	SelectedTables    []aiTableRef    `json:"selectedTables"`
	ScopeConfirmation string          `json:"scopeConfirmation"`
}

// scopeConfirmationPrefix marks an internal response consumed by the chat UI.
// It is deliberately kept out of the natural-language conversation so that a
// user must explicitly approve the AI's narrowed schema before SQL is made.
const scopeConfirmationPrefix = "__DBFOCK_SCOPE_CONFIRMATION__:"

type aiScopeConfirmation struct {
	Step      string       `json:"step"`
	Prompt    string       `json:"prompt"`
	Databases []string     `json:"databases"`
	Tables    []aiTableRef `json:"tables"`
}

func scopeConfirmationMessage(step, prompt string, databases []string, tables []aiTableRef) string {
	if databases == nil {
		databases = []string{}
	}
	if tables == nil {
		tables = []aiTableRef{}
	}
	payload, err := json.Marshal(aiScopeConfirmation{Step: step, Prompt: prompt, Databases: databases, Tables: tables})
	if err != nil {
		return ""
	}
	return scopeConfirmationPrefix + string(payload)
}

type smartQueryRequest struct {
	ConnectionID string `json:"connectionId"`
	SQL          string `json:"sql"`
}

var smartQueryParameterPattern = regexp.MustCompile(`:[A-Za-z][A-Za-z0-9_]*\b`)
var smartQueryWherePattern = regexp.MustCompile(`(?i)\bWHERE\b`)

func (a *API) createSmartQuery(w http.ResponseWriter, r *http.Request) {
	var in smartQueryRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	statement := strings.TrimSpace(in.SQL)
	if !strings.HasPrefix(strings.ToUpper(statement), "SELECT") || !smartQueryWherePattern.MatchString(statement) {
		fail(w, fmt.Errorf("smart queries require a SELECT statement with a WHERE clause"))
		return
	}
	if strings.Contains(strings.TrimSuffix(statement, ";"), ";") {
		fail(w, fmt.Errorf("smart queries accept one SQL statement only"))
		return
	}
	if _, err := a.connections.GetDecrypted(r.Context(), in.ConnectionID); err != nil {
		fail(w, err)
		return
	}
	s, err := a.ai.Get(r.Context())
	if err != nil {
		fail(w, fmt.Errorf("configure an AI provider first: %w", err))
		return
	}

	run, err := ai.NewAuditRun("Create Smart Query")
	if err != nil {
		fail(w, err)
		return
	}
	prompt := "Original successful SQL (preserve its read-only intent and result semantics):\\n" + statement
	system := "You turn a successful MySQL SELECT with WHERE filters into a reusable, read-only Smart Query. Do not inspect, infer, request, or return database schema or parameter types. Preserve the original query semantics. Replace user-editable WHERE literal values with named placeholders and keep the original literal as defaultValue. Each placeholder key MUST be the exact column identifier immediately to the left of that filter, in uppercase and without a table alias. For example, WHERE ID_EMPRESA_PAI_LYRS = 16 must become WHERE ID_EMPRESA_PAI_LYRS = :ID_EMPRESA_PAI_LYRS, with key ID_EMPRESA_PAI_LYRS. For an IN filter, use that column name as the one placeholder inside the parentheses and set defaultValue to a comma-separated string, such as 1,2,3. Write a concise title and description in the user's language. Return only valid JSON with this exact shape: {\\\"title\\\":\\\"...\\\",\\\"description\\\":\\\"...\\\",\\\"sql\\\":\\\"SELECT ... WHERE ... :COLUMN_NAME\\\",\\\"parameters\\\":[{\\\"key\\\":\\\"COLUMN_NAME\\\",\\\"defaultValue\\\":\\\"...\\\"}]}. Never produce write SQL or multiple statements."
	raw, err := a.ai.ChatWithAudit(r.Context(), s, run, "smart_query", system, prompt)
	if err != nil {
		fail(w, err)
		return
	}
	var smart models.SmartQuery
	if !decodeAIJSON(raw, &smart) || !validSmartQuery(smart) {
		fail(w, fmt.Errorf("AI returned an invalid smart query"))
		return
	}
	respond(w, http.StatusOK, smart)
}

func validSmartQuery(query models.SmartQuery) bool {
	if strings.TrimSpace(query.Title) == "" || strings.TrimSpace(query.Description) == "" || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query.SQL)), "SELECT") || strings.Contains(strings.TrimSuffix(strings.TrimSpace(query.SQL), ";"), ";") || len(query.Parameters) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, parameter := range query.Parameters {
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`).MatchString(parameter.Key) || seen[parameter.Key] || !strings.Contains(query.SQL, ":"+parameter.Key) {
			return false
		}
		seen[parameter.Key] = true
	}
	for _, placeholder := range smartQueryParameterPattern.FindAllString(query.SQL, -1) {
		if !seen[strings.TrimPrefix(placeholder, ":")] {
			return false
		}
	}
	return true
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	maxAISelectedTables = 12
	maxAIChatMessages   = 20
	maxAIChatChars      = 16_000
	// The workflow can make up to six provider requests. This is intentionally
	// longer than a single provider request so a slow early selection does not
	// cancel the final SQL validation.
	aiWorkflowTimeout = 5 * time.Minute
)

type aiTableRef struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

type aiTableColumns struct {
	Database string   `json:"database"`
	Table    string   `json:"table"`
	Columns  []string `json:"columns"`
}

type aiRelationship struct {
	Database   string `json:"database"`
	FromTable  string `json:"fromTable"`
	FromColumn string `json:"fromColumn"`
	ToDatabase string `json:"toDatabase"`
	ToTable    string `json:"toTable"`
	ToColumn   string `json:"toColumn"`
}

type aiDatabaseSelection struct {
	Databases []string `json:"databases"`
}
type aiTableSelection struct {
	Tables []aiTableRef `json:"tables"`
}
type aiColumnSelection struct {
	Columns []aiTableColumns `json:"columns"`
}
type aiRelationshipSelection struct {
	Relationships []aiRelationship `json:"relationships"`
}
type aiGeneration struct {
	SQL    string `json:"sql"`
	Answer string `json:"answer"`
}
type aiValidation struct {
	Valid  bool   `json:"valid"`
	SQL    string `json:"sql"`
	Answer string `json:"answer"`
}

const aiJSONOnly = "Return only valid JSON. Do not use Markdown, prose outside JSON, or identifiers that are not in the supplied list."

// aiConversation keeps the context local to the workspace tab while bounding
// the amount of untrusted client input sent to the provider. The current prompt
// is always kept separate so every workflow stage has an unambiguous request.
func aiConversation(history []aiChatMessage, prompt string) string {
	if len(history) > maxAIChatMessages {
		history = history[len(history)-maxAIChatMessages:]
	}

	if len(history) > 0 {
		last := history[len(history)-1]
		if last.Role == "user" && strings.TrimSpace(last.Content) == strings.TrimSpace(prompt) {
			history = history[:len(history)-1]
		}
	}

	lines := make([]string, 0, len(history))
	remaining := maxAIChatChars
	for i := len(history) - 1; i >= 0 && remaining > 0; i-- {
		message := history[i]
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > remaining {
			content = content[len(content)-remaining:]
		}
		lines = append([]string{strings.ToUpper(message.Role[:1]) + message.Role[1:] + ": " + content}, lines...)
		remaining -= len(content)
	}
	if len(lines) == 0 {
		return "User request: " + prompt
	}
	return "Conversation context:\n" + strings.Join(lines, "\n") + "\n\nCurrent user request: " + prompt
}

func decodeAIJSON(raw string, target any) bool {
	raw = strings.TrimSpace(raw)
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return false
	}
	return json.Unmarshal([]byte(raw[start:end+1]), target) == nil
}

func sameAIName(left, right string) bool { return strings.EqualFold(left, right) }

func canonicalDatabase(names []string, candidate string) string {
	for _, name := range names {
		if sameAIName(name, candidate) {
			return name
		}
	}
	return ""
}

func selectedDatabases(selection aiDatabaseSelection, available []string, fallback string) []string {
	selected, seen := []string{}, map[string]bool{}
	for _, candidate := range selection.Databases {
		if name := canonicalDatabase(available, candidate); name != "" && !seen[name] {
			selected, seen[name] = append(selected, name), true
		}
	}
	if len(selected) > 0 {
		return selected
	}
	if name := canonicalDatabase(available, fallback); name != "" {
		return []string{name}
	}
	if len(available) > 0 {
		return []string{available[0]}
	}
	return nil
}

// requestedDatabases accepts only identifiers returned by the connection. It
// keeps a manually scoped request from accidentally broadening back to every
// accessible database when a stale browser selection is submitted.
func requestedDatabases(requested, available []string) []string {
	selected, seen := []string{}, map[string]bool{}
	for _, candidate := range requested {
		if name := canonicalDatabase(available, candidate); name != "" && !seen[name] {
			selected, seen[name] = append(selected, name), true
		}
	}
	return selected
}

func canonicalTable(available []aiTableRef, candidate aiTableRef) (aiTableRef, bool) {
	for _, table := range available {
		if sameAIName(table.Database, candidate.Database) && sameAIName(table.Table, candidate.Table) {
			return table, true
		}
	}
	return aiTableRef{}, false
}

func selectedTables(selection aiTableSelection, available []aiTableRef, prompt string) []aiTableRef {
	selected, seen := []aiTableRef{}, map[aiTableRef]bool{}
	for _, candidate := range selection.Tables {
		if table, ok := canonicalTable(available, candidate); ok && !seen[table] {
			selected, seen[table] = append(selected, table), true
		}
	}
	if len(selected) > 0 {
		return selected
	}
	words := promptIdentifiers(prompt)
	for _, table := range available {
		if words[strings.ToLower(table.Table)] {
			selected = append(selected, table)
		}
	}
	if len(selected) == 0 {
		selected = available
	}
	if len(selected) > maxAISelectedTables {
		selected = selected[:maxAISelectedTables]
	}
	return selected
}

func requestedTables(requested, available []aiTableRef) []aiTableRef {
	selected, seen := []aiTableRef{}, map[aiTableRef]bool{}
	for _, candidate := range requested {
		if table, ok := canonicalTable(available, candidate); ok && !seen[table] {
			selected, seen[table] = append(selected, table), true
		}
	}
	return selected
}

func selectedColumns(selection aiColumnSelection, tables []aiTableRef, structures map[aiTableRef]*models.TableStructure) map[aiTableRef][]string {
	selected := map[aiTableRef][]string{}
	for _, candidate := range selection.Columns {
		table, ok := canonicalTable(tables, aiTableRef{Database: candidate.Database, Table: candidate.Table})
		if !ok || structures[table] == nil {
			continue
		}
		for _, name := range candidate.Columns {
			for _, column := range structures[table].Columns {
				if sameAIName(column.Name, name) && !containsAIName(selected[table], column.Name) {
					selected[table] = append(selected[table], column.Name)
				}
			}
		}
	}
	for _, table := range tables {
		if len(selected[table]) > 0 || structures[table] == nil {
			continue
		}
		for _, column := range structures[table].Columns {
			selected[table] = append(selected[table], column.Name)
		}
	}
	return selected
}

func containsAIName(names []string, candidate string) bool {
	for _, name := range names {
		if sameAIName(name, candidate) {
			return true
		}
	}
	return false
}

func availableRelationships(tables []aiTableRef, structures map[aiTableRef]*models.TableStructure) []aiRelationship {
	relationships := []aiRelationship{}
	for _, table := range tables {
		structure := structures[table]
		if structure == nil {
			continue
		}
		for _, foreignKey := range structure.ForeignKeys {
			for _, target := range tables {
				if sameAIName(table.Database, target.Database) && sameAIName(foreignKey.ReferencedTable, target.Table) {
					relationships = append(relationships, aiRelationship{Database: table.Database, FromTable: table.Table, FromColumn: foreignKey.Column, ToDatabase: target.Database, ToTable: target.Table, ToColumn: foreignKey.ReferencedColumn})
					break
				}
			}
		}
	}
	return relationships
}

func selectedRelationships(selection aiRelationshipSelection, available []aiRelationship) []aiRelationship {
	selected := []aiRelationship{}
	for _, candidate := range selection.Relationships {
		for _, relationship := range available {
			if sameAIName(candidate.Database, relationship.Database) && sameAIName(candidate.FromTable, relationship.FromTable) && sameAIName(candidate.FromColumn, relationship.FromColumn) && sameAIName(candidate.ToDatabase, relationship.ToDatabase) && sameAIName(candidate.ToTable, relationship.ToTable) && sameAIName(candidate.ToColumn, relationship.ToColumn) {
				selected = append(selected, relationship)
				break
			}
		}
	}
	if len(selected) == 0 {
		return available
	}
	return selected
}

func aiTableList(tables []aiTableRef) string {
	var out strings.Builder
	for _, table := range tables {
		fmt.Fprintf(&out, "\n- `%s`.`%s`", table.Database, table.Table)
	}
	return out.String()
}

func aiColumnList(tables []aiTableRef, structures map[aiTableRef]*models.TableStructure, selected map[aiTableRef][]string, relationships []aiRelationship) string {
	needed := map[aiTableRef][]string{}
	includeAllColumns := selected == nil
	for table, columns := range selected {
		needed[table] = append(needed[table], columns...)
	}
	for _, relationship := range relationships {
		from, _ := canonicalTable(tables, aiTableRef{Database: relationship.Database, Table: relationship.FromTable})
		to, _ := canonicalTable(tables, aiTableRef{Database: relationship.ToDatabase, Table: relationship.ToTable})
		if !containsAIName(needed[from], relationship.FromColumn) {
			needed[from] = append(needed[from], relationship.FromColumn)
		}
		if !containsAIName(needed[to], relationship.ToColumn) {
			needed[to] = append(needed[to], relationship.ToColumn)
		}
	}
	var out strings.Builder
	for _, table := range tables {
		structure := structures[table]
		if structure == nil {
			continue
		}
		columns := []string{}
		for _, column := range structure.Columns {
			if includeAllColumns || containsAIName(needed[table], column.Name) {
				columns = append(columns, "`"+column.Name+"` "+column.ColumnType)
			}
		}
		fmt.Fprintf(&out, "\n- `%s`.`%s` (%s)", table.Database, table.Table, strings.Join(columns, ", "))
	}
	if len(relationships) > 0 {
		out.WriteString("\nRelationships:")
		for _, relationship := range relationships {
			fmt.Fprintf(&out, "\n- `%s`.`%s`.`%s` → `%s`.`%s`.`%s`", relationship.Database, relationship.FromTable, relationship.FromColumn, relationship.ToDatabase, relationship.ToTable, relationship.ToColumn)
		}
	}
	return out.String()
}

func promptIdentifiers(prompt string) map[string]bool {
	identifiers := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '$')
	}) {
		identifiers[word] = true
	}
	return identifiers
}

func (a *API) aiChat(w http.ResponseWriter, r *http.Request) {
	var in aiChatRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	message, err := a.generateAIChat(r.Context(), in)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": message})
}

func (a *API) createAIChatJob(w http.ResponseWriter, r *http.Request) {
	var in aiChatRequest
	if err := decode(w, r, &in); err != nil {
		return
	}
	if strings.TrimSpace(in.Prompt) == "" {
		fail(w, fmt.Errorf("prompt is required"))
		return
	}
	job, err := a.repo.CreateAIChatJob(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	go a.runAIChatJob(job.ID, in)
	respond(w, http.StatusAccepted, job)
}

func (a *API) getAIChatJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.repo.GetAIChatJob(r.Context(), repository.LocalUserID, chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, http.StatusOK, job)
}

func (a *API) runAIChatJob(id string, in aiChatRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), aiWorkflowTimeout)
	defer cancel()
	message, err := a.generateAIChat(ctx, in)
	if err != nil {
		a.log.Error("AI chat job failed", "job_id", id, "error", err)
		_ = a.repo.FailAIChatJob(context.Background(), id, err.Error())
		return
	}
	if err := a.repo.CompleteAIChatJob(context.Background(), id, message); err != nil {
		a.log.Error("save AI chat job result", "job_id", id, "error", err)
	}
}

func (a *API) generateAIChat(ctx context.Context, in aiChatRequest) (string, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}
	conversation := aiConversation(in.History, in.Prompt)
	s, err := a.ai.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("configure an AI provider first: %w", err)
	}
	c, err := a.connections.GetDecrypted(ctx, in.ConnectionID)
	if err != nil {
		return "", err
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		return "", err
	}
	databaseName := in.Database
	if databaseName == "" {
		databaseName = c.InitialDatabase
	}
	// Discovery is deliberately progressive: the model first sees database
	// names, then only the selected tables, and only then their columns. This
	// avoids repeatedly sending a connection's complete schema in one request.
	ctx, cancel := context.WithTimeout(ctx, aiWorkflowTimeout)
	defer cancel()
	auditRun, err := ai.NewAuditRun(in.Prompt)
	if err != nil {
		return "", err
	}

	databases, err := p.ListDatabases(ctx, c)
	if err != nil {
		return "", fmt.Errorf("discover accessible databases: %w", err)
	}
	sort.Slice(databases, func(i, j int) bool { return databases[i].Name < databases[j].Name })
	databaseNames := make([]string, 0, len(databases))
	for _, database := range databases {
		databaseNames = append(databaseNames, database.Name)
	}

	selectedDatabaseNames := []string{}
	if in.DatabaseScope == "selected" {
		selectedDatabaseNames = requestedDatabases(in.SelectedDatabases, databaseNames)
		if len(selectedDatabaseNames) == 0 {
			return "", fmt.Errorf("select at least one accessible database")
		}
	} else {
		var databaseChoice aiDatabaseSelection
		raw, chatErr := a.ai.ChatWithAudit(ctx, s, auditRun, "select_database", "You select the database(s) relevant to a MySQL request. Choose only exact names from the supplied list. "+aiJSONOnly, conversation+"\nAccessible databases: "+strings.Join(databaseNames, ", ")+"\nJSON shape: {\"databases\":[\"database\"]}")
		if chatErr != nil {
			return "", chatErr
		}
		_ = decodeAIJSON(raw, &databaseChoice)
		selectedDatabaseNames = selectedDatabases(databaseChoice, databaseNames, databaseName)
	}
	if len(selectedDatabaseNames) == 0 {
		return "No accessible databases were found for this connection.", nil
	}
	// With the default broad scope, pause after each AI recommendation. This is
	// especially useful for connections with many databases and tables: users
	// can approve the concise suggestion or switch to the existing picker.
	if in.DatabaseScope == "all" && in.TableScope == "all" && in.ScopeConfirmation == "" {
		return scopeConfirmationMessage("databases", in.Prompt, selectedDatabaseNames, nil), nil
	}

	availableTables := []aiTableRef{}
	for _, name := range selectedDatabaseNames {
		tables, listErr := p.ListTables(ctx, c, name, false)
		if listErr != nil {
			continue
		}
		sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })
		for _, table := range tables {
			availableTables = append(availableTables, aiTableRef{Database: name, Table: table.Name})
		}
	}
	if len(availableTables) == 0 {
		return "No base tables were found in the selected database.", nil
	}

	selectedTableRefs := []aiTableRef{}
	if in.TableScope == "selected" {
		selectedTableRefs = requestedTables(in.SelectedTables, availableTables)
		if len(selectedTableRefs) == 0 {
			return "", fmt.Errorf("select at least one table from the selected databases")
		}
	} else {
		var tableChoice aiTableSelection
		raw, chatErr := a.ai.ChatWithAudit(ctx, s, auditRun, "select_tables", "You select the smallest set of MySQL tables needed for a request. Choose only exact database and table pairs from the supplied list. "+aiJSONOnly, conversation+"\nCandidate tables:"+aiTableList(availableTables)+"\nJSON shape: {\"tables\":[{\"database\":\"...\",\"table\":\"...\"}]}")
		if chatErr != nil {
			return "", chatErr
		}
		_ = decodeAIJSON(raw, &tableChoice)
		selectedTableRefs = selectedTables(tableChoice, availableTables, in.Prompt)
	}
	if in.TableScope == "all" && in.ScopeConfirmation == "databases" {
		return scopeConfirmationMessage("tables", in.Prompt, selectedDatabaseNames, selectedTableRefs), nil
	}

	structures := map[aiTableRef]*models.TableStructure{}
	for _, table := range selectedTableRefs {
		structure, structureErr := p.GetTableStructure(ctx, c, table.Database, table.Table)
		if structureErr == nil && structure != nil {
			structures[table] = structure
		}
	}
	if len(structures) == 0 {
		return "The selected tables do not expose schema details.", nil
	}

	var columnChoice aiColumnSelection
	raw, err := a.ai.ChatWithAudit(ctx, s, auditRun, "select_columns", "You select only the MySQL columns needed to answer a request. Choose only exact columns from the supplied table definitions. Keep join keys when needed. "+aiJSONOnly, conversation+"\nCandidate columns:"+aiColumnList(selectedTableRefs, structures, nil, nil)+"\nJSON shape: {\"columns\":[{\"database\":\"...\",\"table\":\"...\",\"columns\":[\"...\"]}]}")
	if err != nil {
		return "", err
	}
	_ = decodeAIJSON(raw, &columnChoice)
	selectedColumnNames := selectedColumns(columnChoice, selectedTableRefs, structures)

	availableRelations := availableRelationships(selectedTableRefs, structures)
	var relationshipChoice aiRelationshipSelection
	raw, err = a.ai.ChatWithAudit(ctx, s, auditRun, "select_relationships", "You select which declared foreign-key relationships are needed for a MySQL request. Choose only exact relationships from the supplied list. "+aiJSONOnly, conversation+"\nCandidate relationships:"+aiColumnList(selectedTableRefs, structures, selectedColumnNames, availableRelations)+"\nJSON shape: {\"relationships\":[{\"database\":\"...\",\"fromTable\":\"...\",\"fromColumn\":\"...\",\"toDatabase\":\"...\",\"toTable\":\"...\",\"toColumn\":\"...\"}]}")
	if err != nil {
		return "", err
	}
	_ = decodeAIJSON(raw, &relationshipChoice)
	selectedRelationNames := selectedRelationships(relationshipChoice, availableRelations)
	focusedSchema := aiColumnList(selectedTableRefs, structures, selectedColumnNames, selectedRelationNames)

	var generated aiGeneration
	raw, err = a.ai.ChatWithAudit(ctx, s, auditRun, "generate_sql", "You generate safe MySQL SQL using only the focused schema supplied. Identifiers are literal: never translate, rename, normalize, or invent them; use backticks around identifiers. If the request is informational and needs no SQL, leave sql empty. Answer in the user's language. "+aiJSONOnly, conversation+"\nFocused schema:"+focusedSchema+"\nJSON shape: {\"sql\":\"SQL only, without Markdown\",\"answer\":\"brief explanation\"}")
	if err != nil {
		return "", err
	}
	if !decodeAIJSON(raw, &generated) {
		return "", fmt.Errorf("AI returned an invalid SQL generation response")
	}
	if strings.TrimSpace(generated.SQL) == "" {
		return generated.Answer, nil
	}

	var validation aiValidation
	raw, err = a.ai.ChatWithAudit(ctx, s, auditRun, "validate_sql", "You validate MySQL SQL against the focused schema. Correct the SQL only when necessary. Reject SQL that needs identifiers or relationships not present in the schema. Answer in the user's language. "+aiJSONOnly, conversation+"\nFocused schema:"+focusedSchema+"\nGenerated SQL:\n"+generated.SQL+"\nJSON shape: {\"valid\":true,\"sql\":\"validated SQL only, without Markdown\",\"answer\":\"brief validation result\"}")
	if err != nil {
		return "", err
	}
	if !decodeAIJSON(raw, &validation) {
		return "", fmt.Errorf("AI returned an invalid SQL validation response")
	}
	if !validation.Valid || strings.TrimSpace(validation.SQL) == "" {
		return validation.Answer, nil
	}
	answer := strings.TrimSpace(validation.Answer)
	if answer == "" {
		answer = strings.TrimSpace(generated.Answer)
	}
	if answer != "" {
		answer += "\n\n"
	}
	answer += "```sql\n" + strings.TrimSpace(validation.SQL) + "\n```"
	return answer, nil
}
func (a *API) deleteHistory(w http.ResponseWriter, r *http.Request) {
	if err := a.repo.DeleteHistory(r.Context(), repository.LocalUserID, chi.URLParam(r, "id")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) connection(ctx context.Context) (models.Connection, error) {
	return a.connections.GetDecrypted(ctx, chi.URLParamFromCtx(ctx, "id"))
}
func (a *API) timeout(c models.Connection) time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}
func (a *API) status(id string) string {
	if a.isConnected(id) {
		return "connected"
	}
	return "disconnected"
}
func (a *API) isConnected(id string) bool {
	a.sessionMu.RLock()
	defer a.sessionMu.RUnlock()
	return a.sessions[id]
}
func (a *API) requireConnected(id string) error {
	if !a.isConnected(id) {
		return errDatabaseNotConnected
	}
	return nil
}
func queryInt(r *http.Request, key string, fallback, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil || n < min || n > max {
		return fallback
	}
	return n
}
func operationType(s string) string {
	f := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	if len(f) == 0 {
		return "UNKNOWN"
	}
	return f[0]
}
func isDataMutation(operation string) bool {
	return operation == "INSERT" || operation == "UPDATE" || operation == "DELETE"
}
func isReadOnly(operation string) bool {
	return operation == "SELECT" || operation == "SHOW" || operation == "DESCRIBE" || operation == "EXPLAIN"
}
func (a *API) rollbackPendingTransaction(ctx context.Context, id string) {
	c, err := a.connections.GetDecrypted(ctx, id)
	if err != nil {
		return
	}
	p, err := a.providers.Get(c.Driver)
	if err != nil {
		return
	}
	if transactional, ok := p.(database.TransactionalProvider); ok {
		_ = transactional.RollbackTransaction(ctx, c)
	}
}
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		fail(w, fmt.Errorf("invalid request: %w", err))
		return err
	}
	return nil
}
func respond(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	respond(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}
func (a *API) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		a.log.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}
