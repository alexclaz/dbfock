package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dbfock/database-manager/backend/internal/ai"
	"github.com/dbfock/database-manager/backend/internal/config"
	"github.com/dbfock/database-manager/backend/internal/connections"
	"github.com/dbfock/database-manager/backend/internal/database"
	mysqlprovider "github.com/dbfock/database-manager/backend/internal/database/mysql"
	"github.com/dbfock/database-manager/backend/internal/encryption"
	httpapi "github.com/dbfock/database-manager/backend/internal/http"
	"github.com/dbfock/database-manager/backend/internal/middleware"
	"github.com/dbfock/database-manager/backend/internal/repository"
	"github.com/dbfock/database-manager/backend/migrations"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:desktop/assets
var assets embed.FS

type desktopApp struct {
	server *http.Server
	repo   *repository.Repository
	logger *slog.Logger
}

func main() {
	app, err := newDesktopApp()
	if err != nil {
		slog.Error("desktop startup error", "error", err)
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:       "DBfock",
		Width:       1440,
		Height:      900,
		MinWidth:    1024,
		MinHeight:   680,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   app.start,
		OnShutdown:  app.stop,
	})
	if err != nil {
		slog.Error("desktop stopped", "error", err)
		os.Exit(1)
	}
}

func newDesktopApp() (*desktopApp, error) {
	dataDir, err := desktopDataDir()
	if err != nil {
		return nil, err
	}
	key, err := encryptionKey(dataDir)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	cfg.Env = "desktop"
	cfg.Host = "127.0.0.1"
	cfg.Port = "8080"
	cfg.DatabasePath = filepath.Join(dataDir, "app.db")
	cfg.EncryptionKey = key
	cfg.CORSAllowedOrigins = []string{
		"http://127.0.0.1:1420",
		"http://localhost:1420",
		"http://wails.localhost",
		"https://wails.localhost",
		"wails://wails",
	}

	cipher, err := encryption.New(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	repo, err := repository.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := repo.MigrateFS(context.Background(), migrations.Files); err != nil {
		repo.Close()
		return nil, err
	}

	providers := database.NewRegistry()
	providers.Register("mysql", mysqlprovider.New(cfg.MaxOpenConnections))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	api := httpapi.New(cfg, connections.New(repo, cipher, providers), providers, repo, ai.New(repo, cipher), logger)
	handler := middleware.RateLimit(120, time.Minute)(middleware.CORS(cfg.CORSAllowedOrigins)(api.Router()))

	return &desktopApp{
		server: &http.Server{
			Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
			Handler:           http.MaxBytesHandler(handler, cfg.MaxRequestBodyBytes),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		repo:   repo,
		logger: logger,
	}, nil
}

func (a *desktopApp) start(_ context.Context) {
	go func() {
		a.logger.Info("desktop API listening", "address", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("desktop API stopped", "error", err)
		}
	}()
}

func (a *desktopApp) stop(_ context.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		a.logger.Error("desktop API shutdown failed", "error", err)
	}
	if err := a.repo.Close(); err != nil {
		a.logger.Error("desktop database close failed", "error", err)
	}
}

func desktopDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve desktop data directory: %w", err)
	}
	dir := filepath.Join(base, "DBfock")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create desktop data directory: %w", err)
	}
	return dir, nil
}

func encryptionKey(dataDir string) (string, error) {
	keyPath := filepath.Join(dataDir, ".encryption-key")
	key, err := os.ReadFile(keyPath)
	if err == nil {
		if len(key) == 0 {
			return "", errors.New("desktop encryption key is empty")
		}
		return string(key), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read desktop encryption key: %w", err)
	}

	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate desktop encryption key: %w", err)
	}
	key = []byte(hex.EncodeToString(bytes))
	file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		key, err = os.ReadFile(keyPath)
		if err != nil {
			return "", fmt.Errorf("read concurrently created desktop encryption key: %w", err)
		}
		return string(key), nil
	}
	if err != nil {
		return "", fmt.Errorf("create desktop encryption key: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		return "", fmt.Errorf("write desktop encryption key: %w", err)
	}
	return string(key), nil
}
