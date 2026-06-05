package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Loonstagram/internal/app"
	"Loonstagram/internal/automation"
	"Loonstagram/internal/cache"
	"Loonstagram/internal/discord"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/mediacache"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: app.ParseLogLevel(cfg.LogLevel),
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := cache.Open(ctx, cfg.DatabasePath)
	if err != nil {
		logger.Error("open cache", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	go store.StartCleanup(ctx, 10*time.Minute, logger)

	mediaCache, err := mediacache.Open(cfg.MediaCachePath, cfg.MaxMediaBytes)
	if err != nil {
		logger.Error("open media cache", "error", err)
		os.Exit(1)
	}

	scraper := instagram.NewClient(instagram.ClientConfig{
		Timeout:      cfg.HTTPClientTimeout,
		MaxBodyBytes: cfg.MaxFetchBytes,
	})
	profiles := instagram.NewProfileClient(instagram.ProfileClientConfig{
		Timeout:      cfg.HTTPClientTimeout,
		MaxBodyBytes: cfg.MaxFetchBytes,
		AppID:        cfg.InstagramWebAppID,
		SessionID:    cfg.InstagramSessionID,
	})
	poller := automation.NewPoller(automation.Options{
		Store:         store,
		Profiles:      profiles,
		Posts:         scraper,
		Discord:       discord.NewClient(cfg.HTTPClientTimeout),
		PublicBaseURL: cfg.PublicBaseURL,
		SecretKey:     cfg.AdminToken,
		Interval:      cfg.AutomationPollInterval,
		CacheTTL:      cfg.CacheSuccessTTL,
		Logger:        logger,
	})
	go poller.Run(ctx)

	handler, err := app.NewHTTPHandler(app.HTTPHandlerOptions{
		Config:  cfg,
		Store:   store,
		Media:   mediaCache,
		Scraper: scraper,
		Logger:  logger,
	})
	if err != nil {
		logger.Error("create handler", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown server", "error", err)
		}
	}()

	logger.Info("starting Loonstagram", "addr", cfg.ListenAddr, "public_base_url", cfg.PublicBaseURL)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
