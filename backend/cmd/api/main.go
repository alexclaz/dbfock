package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dbfock/database-manager/backend/internal/ai"
	"github.com/dbfock/database-manager/backend/internal/backup"
	"github.com/dbfock/database-manager/backend/internal/config"
	"github.com/dbfock/database-manager/backend/internal/connections"
	"github.com/dbfock/database-manager/backend/internal/database"
	mysqlprovider "github.com/dbfock/database-manager/backend/internal/database/mysql"
	"github.com/dbfock/database-manager/backend/internal/encryption"
	httpapi "github.com/dbfock/database-manager/backend/internal/http"
	"github.com/dbfock/database-manager/backend/internal/middleware"
	"github.com/dbfock/database-manager/backend/internal/repository"
	"github.com/dbfock/database-manager/backend/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}
	cipher, err := encryption.New(cfg.EncryptionKey)
	if err != nil {
		slog.Error("encryption configuration error", "error", err)
		os.Exit(1)
	}
	repo, err := repository.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("database error", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	if err = repo.MigrateFS(context.Background(), migrations.Files); err != nil {
		slog.Error("migration error", "error", err)
		os.Exit(1)
	}
	providers := database.NewRegistry()
	providers.Register("mysql", mysqlprovider.New(cfg.MaxOpenConnections))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	api := httpapi.New(cfg, connections.New(repo, cipher, providers), providers, repo, ai.New(repo, cipher), backup.New(repo, cipher), logger)
	handler := middleware.RateLimit(120, time.Minute)(middleware.CORS(cfg.CORSAllowedOrigins)(api.Router()))
	server := &http.Server{Addr: cfg.Host + ":" + cfg.Port, Handler: http.MaxBytesHandler(handler, cfg.MaxRequestBodyBytes), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		logger.Info("API listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
