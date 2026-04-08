// internal/config/config.go
package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	AppEnv      string
	AppPort     string
	AppURL      string
	FrontendURL string
	CookieDomain string

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// JWT
	JWTAccessSecret   string
	JWTRefreshSecret  string
	JWTDocumentSecret string
	JWTAccessExpiry   time.Duration
	JWTRefreshExpiry  time.Duration
	JWTDocumentExpiry time.Duration

	// CORS
	CORSAllowedOrigin string

	// Logging
	LogLevel string

	// Rate Limiting
	RateLimitRequests int
	RateLimitWindow   time.Duration

	// Email — Resend
	ResendAPIKey   string // re_xxxxxxxxxxxx
	ResendFromAddr string // e.g. "Docflow <invites@docflow.asia>"
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment directly")
	}

	return &Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		AppPort:     getEnv("APP_PORT", "8080"),
		AppURL:      getEnv("APP_URL", "http://localhost:8080"),
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:5173"),
		CookieDomain: getEnv("COOKIE_DOMAIN", ""),

		DatabaseURL: mustGetEnv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", ""),

		JWTAccessSecret:   mustGetEnv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret:  mustGetEnv("JWT_REFRESH_SECRET"),
		JWTDocumentSecret: mustGetEnv("JWT_DOCUMENT_SECRET"),

		JWTAccessExpiry:   mustParseDuration("JWT_ACCESS_EXPIRY", "15m"),
		JWTRefreshExpiry:  mustParseDuration("JWT_REFRESH_EXPIRY", "168h"),
		JWTDocumentExpiry: mustParseDuration("JWT_DOCUMENT_EXPIRY", "1h"),

		CORSAllowedOrigin: getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
		RateLimitRequests: mustParseInt("RATE_LIMIT_REQUESTS", 60),
		RateLimitWindow:   mustParseDuration("RATE_LIMIT_WINDOW", "1m"),

		LogLevel: getEnv("LOG_LEVEL", "debug"),

		// Email — required in production, optional in development
		// (if missing, invitation emails silently fail but rows are still created)
		ResendAPIKey:   getEnv("RESEND_API_KEY", ""),
		ResendFromAddr: getEnv("RESEND_FROM_ADDR", "Docflow <invites@docflow.asia>"),
	}
}

func (c *Config) IsDevelopment() bool {
	return c.AppEnv == "development"
}

// IsEmailConfigured returns true when the Resend API key is set.
// In development you can omit the key — invitations are created but not emailed.
func (c *Config) IsEmailConfigured() bool {
	return c.ResendAPIKey != ""
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return val
}

func mustParseDuration(key, fallback string) time.Duration {
	val := getEnv(key, fallback)
	d, err := time.ParseDuration(val)
	if err != nil {
		log.Fatalf("invalid duration for %s: %s", key, val)
	}
	return d
}

func mustParseInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		log.Fatalf("invalid integer for %s: %s", key, val)
	}
	return n
}