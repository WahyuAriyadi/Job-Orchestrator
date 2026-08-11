package config

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration, loaded from environment variables
// so the service is 12-factor friendly (works the same locally, in Docker,
// or on Cloud Run where env vars are injected).
type Config struct {
	DatabaseURL      string
	HTTPPort         string
	PollInterval     int // seconds between scheduler ticks
	LeaderLockID     int64
	LeaderRetryEvery int // seconds between leader-election attempts
}

func Load() Config {
	return Config{
		DatabaseURL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/orchestrator?sslmode=disable"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		PollInterval:     getEnvInt("POLL_INTERVAL_SECONDS", 5),
		LeaderLockID:     int64(getEnvInt("LEADER_LOCK_ID", 8675309)), // arbitrary fixed advisory-lock key for this app
		LeaderRetryEvery: getEnvInt("LEADER_RETRY_SECONDS", 3),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
