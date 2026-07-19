package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                  string
	Host                 string
	Port                 string
	DatabasePath         string
	EncryptionKey        string
	CORSAllowedOrigins   []string
	DefaultQueryTimeout  time.Duration
	MaxQueryRows         int
	MaxOpenConnections   int
	MaxConcurrentQueries int
	MaxRequestBodyBytes  int64
}

func Load() (Config, error) {
	timeout, err := integer("DEFAULT_QUERY_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	maxRows, err := integer("MAX_QUERY_ROWS", 200)
	if err != nil {
		return Config{}, err
	}
	maxOpen, err := integer("MAX_OPEN_CONNECTIONS", 10)
	if err != nil {
		return Config{}, err
	}
	maxConcurrent, err := integer("MAX_CONCURRENT_QUERIES", 4)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Env: value("APP_ENV", "development"), Host: value("APP_HOST", ""), Port: value("APP_PORT", "8080"),
		DatabasePath:        value("APP_DATABASE_PATH", "./data/app.db"),
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
		CORSAllowedOrigins:  strings.Split(value("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ","),
		DefaultQueryTimeout: time.Duration(timeout) * time.Second, MaxQueryRows: maxRows,
		MaxOpenConnections: maxOpen, MaxConcurrentQueries: maxConcurrent,
		MaxRequestBodyBytes: 1 << 20,
	}, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func integer(key string, fallback int) (int, error) {
	v := value(key, strconv.Itoa(fallback))
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}
