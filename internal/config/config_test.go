package config_test

import (
	"log/slog"
	"testing"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/config"
)

func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	t.Setenv("APP_PORT", "8080")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("STORAGE_MODE", "postgres")
	t.Setenv("RESERVATION_TTL_SECONDS", "300")
	t.Setenv("SWEEPER_INTERVAL_SECONDS", "10")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "flashgate")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "flashgate")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("DB_MAX_CONNS", "25")
	t.Setenv("DB_MIN_CONNS", "5")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.AppPort != "8080" {
		t.Errorf("expected AppPort 8080, got %s", cfg.AppPort)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("expected LogLevel debug, got %v", cfg.LogLevel)
	}
	if cfg.DB.MaxConns != 25 || cfg.DB.MinConns != 5 {
		t.Errorf("unexpected pool conns: max=%d min=%d", cfg.DB.MaxConns, cfg.DB.MinConns)
	}
	wantURL := "postgres://flashgate:secret@localhost:5432/flashgate?sslmode=disable"
	if got := cfg.DB.DatabaseURL(); got != wantURL {
		t.Errorf("expected DatabaseURL %q, got %q", wantURL, got)
	}
}

func TestLoad_MissingRequiredDBFields(t *testing.T) {
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when required DB env vars are missing")
	}
}

func TestLoad_InvalidPortAndPool(t *testing.T) {
	t.Setenv("APP_PORT", "99999")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_USER", "flashgate")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "flashgate")
	t.Setenv("DB_MAX_CONNS", "5")
	t.Setenv("DB_MIN_CONNS", "10")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected validation error for invalid port and DB_MIN_CONNS > DB_MAX_CONNS")
	}
}
