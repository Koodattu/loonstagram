package app

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultMaxFetchBytes = 2 * 1024 * 1024
const defaultMaxMediaBytes = 64 * 1024 * 1024

type Config struct {
	PublicBaseURL                  string
	ListenAddr                     string
	DatabasePath                   string
	CacheSuccessTTL                time.Duration
	CacheNegativeTTL               time.Duration
	CacheBlockedTTL                time.Duration
	HTTPClientTimeout              time.Duration
	MediaProxyMode                 string
	EnableInstagramGraphQLFallback bool
	InstagramOEmbedAccessToken     string
	InstagramSessionID             string
	InstagramWebAppID              string
	AutomationPollInterval         time.Duration
	AdminToken                     string
	DiscordClientID                string
	DiscordClientSecret            string
	DiscordRedirectURL             string
	LogLevel                       string
	MaxFetchBytes                  int64
	MediaCachePath                string
	MaxMediaBytes                  int64
}

func LoadConfig() (Config, error) {
	cfg := Config{
		PublicBaseURL:                  strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/"),
		ListenAddr:                     envString("LISTEN_ADDR", ":3000"),
		DatabasePath:                   strings.TrimSpace(os.Getenv("DATABASE_PATH")),
		CacheSuccessTTL:                envDuration("CACHE_SUCCESS_TTL", 6*time.Hour),
		CacheNegativeTTL:               envDuration("CACHE_NEGATIVE_TTL", 15*time.Minute),
		CacheBlockedTTL:                envDuration("CACHE_BLOCKED_TTL", 5*time.Minute),
		HTTPClientTimeout:              envDuration("HTTP_CLIENT_TIMEOUT", 8*time.Second),
		MediaProxyMode:                 envString("MEDIA_PROXY_MODE", "redirect"),
		EnableInstagramGraphQLFallback: envBool("ENABLE_INSTAGRAM_GQL_FALLBACK", false),
		InstagramOEmbedAccessToken:     os.Getenv("INSTAGRAM_OEMBED_ACCESS_TOKEN"),
		InstagramSessionID:             strings.TrimSpace(os.Getenv("INSTAGRAM_SESSION_ID")),
		InstagramWebAppID:              envString("INSTAGRAM_WEB_APP_ID", "936619743392459"),
		AutomationPollInterval:         envDuration("AUTOMATION_POLL_INTERVAL", 30*time.Minute),
		AdminToken:                     strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		DiscordClientID:                strings.TrimSpace(os.Getenv("DISCORD_CLIENT_ID")),
		DiscordClientSecret:            strings.TrimSpace(os.Getenv("DISCORD_CLIENT_SECRET")),
		DiscordRedirectURL:             strings.TrimSpace(os.Getenv("DISCORD_REDIRECT_URL")),
		LogLevel:                       envString("LOG_LEVEL", "info"),
		MaxFetchBytes:                  envInt64("MAX_FETCH_BYTES", defaultMaxFetchBytes),
		MediaCachePath:                strings.TrimSpace(os.Getenv("MEDIA_CACHE_PATH")),
		MaxMediaBytes:                  envInt64("MAX_MEDIA_BYTES", defaultMaxMediaBytes),
	}

	if cfg.PublicBaseURL == "" {
		return cfg, errors.New("PUBLIC_BASE_URL is required")
	}
	parsed, err := url.Parse(cfg.PublicBaseURL)
	if err != nil {
		return cfg, fmt.Errorf("PUBLIC_BASE_URL is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return cfg, errors.New("PUBLIC_BASE_URL must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return cfg, errors.New("PUBLIC_BASE_URL must be an absolute origin without credentials, query, or fragment")
	}

	if cfg.DatabasePath == "" {
		return cfg, errors.New("DATABASE_PATH is required")
	}
	if cfg.MediaCachePath == "" {
		cfg.MediaCachePath = filepath.Join(filepath.Dir(cfg.DatabasePath), "media-cache")
	}

	switch cfg.MediaProxyMode {
	case "redirect", "stream":
	default:
		return cfg, errors.New("MEDIA_PROXY_MODE must be redirect or stream")
	}

	if cfg.EnableInstagramGraphQLFallback {
		return cfg, errors.New("ENABLE_INSTAGRAM_GQL_FALLBACK is reserved for later fallback support and must be false for this MVP")
	}

	if cfg.MaxFetchBytes <= 0 {
		return cfg, errors.New("MAX_FETCH_BYTES must be positive")
	}
	if cfg.MaxMediaBytes <= 0 {
		return cfg, errors.New("MAX_MEDIA_BYTES must be positive")
	}
	if cfg.AutomationPollInterval <= 0 {
		return cfg, errors.New("AUTOMATION_POLL_INTERVAL must be positive")
	}
	if (cfg.DiscordClientID == "") != (cfg.DiscordClientSecret == "") {
		return cfg, errors.New("DISCORD_CLIENT_ID and DISCORD_CLIENT_SECRET must be configured together")
	}
	if cfg.DiscordRedirectURL != "" {
		parsed, err := url.Parse(cfg.DiscordRedirectURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return cfg, errors.New("DISCORD_REDIRECT_URL must be an absolute URL without credentials, query, or fragment")
		}
	}

	return cfg, nil
}

func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
