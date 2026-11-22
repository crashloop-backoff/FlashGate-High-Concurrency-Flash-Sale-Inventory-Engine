package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds validated application configuration loaded from environment variables.
type Config struct {
	AppEnv                 string
	AppPort                string
	LogLevel               slog.Level
	StorageMode            string
	ReservationTTLSeconds  int
	SweeperIntervalSeconds int
	DB                     DBConfig
}

// DBConfig holds PostgreSQL connection pool configuration.
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
	MinConns int32
}

// DatabaseURL returns a PostgreSQL connection URI with properly escaped credentials.
func (d DBConfig) DatabaseURL() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.User, d.Password),
		Host:   net.JoinHostPort(d.Host, d.Port),
		Path:   d.Name,
	}
	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// Load reads configuration from environment variables and validates all constraints.
func Load() (Config, error) {
	var errs []error

	appEnv := getEnvOrDefault("APP_ENV", "local")
	appPort := getEnvOrDefault("APP_PORT", "8080")
	if portNum, err := strconv.Atoi(appPort); err != nil || portNum < 1 || portNum > 65535 {
		errs = append(errs, fmt.Errorf("APP_PORT must be a valid port (1-65535), got %q", appPort))
	}

	logLevel, err := parseLogLevel(getEnvOrDefault("LOG_LEVEL", "info"))
	if err != nil {
		errs = append(errs, err)
	}

	storageMode := strings.ToLower(getEnvOrDefault("STORAGE_MODE", "postgres"))
	if storageMode != "postgres" && storageMode != "redis" {
		errs = append(errs, fmt.Errorf("STORAGE_MODE must be 'postgres' or 'redis', got %q", storageMode))
	}

	resTTL, err := getEnvInt("RESERVATION_TTL_SECONDS", 300)
	if err != nil || resTTL <= 0 {
		errs = append(errs, fmt.Errorf("RESERVATION_TTL_SECONDS must be a positive integer"))
	}

	sweeperInterval, err := getEnvInt("SWEEPER_INTERVAL_SECONDS", 10)
	if err != nil || sweeperInterval <= 0 {
		errs = append(errs, fmt.Errorf("SWEEPER_INTERVAL_SECONDS must be a positive integer"))
	}

	dbHost := requireEnv("DB_HOST", &errs)
	dbPort := getEnvOrDefault("DB_PORT", "5432")
	if portNum, portErr := strconv.Atoi(dbPort); portErr != nil || portNum < 1 || portNum > 65535 {
		errs = append(errs, fmt.Errorf("DB_PORT must be a valid port (1-65535), got %q", dbPort))
	}
	dbUser := requireEnv("DB_USER", &errs)
	dbPassword := requireEnv("DB_PASSWORD", &errs)
	dbName := requireEnv("DB_NAME", &errs)
	dbSSLMode := getEnvOrDefault("DB_SSLMODE", "disable")

	maxConns, err := getEnvInt("DB_MAX_CONNS", 25)
	if err != nil || maxConns <= 0 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS must be a positive integer"))
	}

	minConns, err := getEnvInt("DB_MIN_CONNS", 5)
	if err != nil || minConns < 0 {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS must be a non-negative integer"))
	}

	if maxConns > 0 && minConns > maxConns {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS (%d) cannot exceed DB_MAX_CONNS (%d)", minConns, maxConns))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}

	return Config{
		AppEnv:                 appEnv,
		AppPort:                appPort,
		LogLevel:               logLevel,
		StorageMode:            storageMode,
		ReservationTTLSeconds:  resTTL,
		SweeperIntervalSeconds: sweeperInterval,
		DB: DBConfig{
			Host:     dbHost,
			Port:     dbPort,
			User:     dbUser,
			Password: dbPassword,
			Name:     dbName,
			SSLMode:  dbSSLMode,
			MaxConns: int32(maxConns),
			MinConns: int32(minConns),
		},
	}, nil
}

func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string, errs *[]error) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func getEnvInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error, got %q", raw)
	}
}
