package app

import (
	"log/slog"
	"net/http"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/httpx"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/mediacache"
)

type HTTPHandlerOptions struct {
	Config   Config
	Store    *cache.Store
	Media    *mediacache.Store
	Scraper  *instagram.Client
	Profiles *instagram.ProfileClient
	Logger   *slog.Logger
}

func NewHTTPHandler(opts HTTPHandlerOptions) (http.Handler, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	handlers, err := httpx.NewHandlers(httpx.Options{
		PublicBaseURL:       opts.Config.PublicBaseURL,
		CacheSuccessTTL:     opts.Config.CacheSuccessTTL,
		CacheNegativeTTL:    opts.Config.CacheNegativeTTL,
		CacheBlockedTTL:     opts.Config.CacheBlockedTTL,
		MediaProxyMode:      opts.Config.MediaProxyMode,
		AdminToken:          opts.Config.AdminToken,
		DiscordClientID:     opts.Config.DiscordClientID,
		DiscordClientSecret: opts.Config.DiscordClientSecret,
		DiscordRedirectURL:  opts.Config.DiscordRedirectURL,
		Store:               opts.Store,
		MediaCache:          opts.Media,
		Scraper:             opts.Scraper,
		Profiles:            opts.Profiles,
		Logger:              opts.Logger,
	})
	if err != nil {
		return nil, err
	}

	return handlers.Routes(), nil
}
