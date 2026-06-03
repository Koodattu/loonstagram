package automation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"instafix/internal/cache"
	"instafix/internal/discord"
	"instafix/internal/instagram"
	"instafix/internal/secret"
)

const (
	defaultPollInterval = 5 * time.Minute
	defaultFetchLimit   = 12
	maxPostsPerPoll     = 5
)

type ProfileFetcher interface {
	FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error)
}

type DiscordSender interface {
	SendWebhook(ctx context.Context, webhookURL, content string) error
}

type Poller struct {
	store         *cache.Store
	profiles      ProfileFetcher
	discord       DiscordSender
	publicBaseURL string
	secretKey     string
	interval      time.Duration
	logger        *slog.Logger
}

type Options struct {
	Store         *cache.Store
	Profiles      ProfileFetcher
	Discord       DiscordSender
	PublicBaseURL string
	SecretKey     string
	Interval      time.Duration
	Logger        *slog.Logger
}

func NewPoller(opts Options) *Poller {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Discord == nil {
		opts.Discord = discord.NewClient(8 * time.Second)
	}
	return &Poller{
		store:         opts.Store,
		profiles:      opts.Profiles,
		discord:       opts.Discord,
		publicBaseURL: strings.TrimRight(opts.PublicBaseURL, "/"),
		secretKey:     opts.SecretKey,
		interval:      interval,
		logger:        logger,
	}
}

func (p *Poller) Run(ctx context.Context) {
	if p == nil || p.store == nil || p.profiles == nil || p.discord == nil {
		return
	}

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			p.checkAndLog(ctx)
			timer.Reset(p.interval)
		}
	}
}

func (p *Poller) CheckOnce(ctx context.Context) error {
	if p == nil || p.store == nil || p.profiles == nil || p.discord == nil {
		return nil
	}
	return p.check(ctx)
}

func (p *Poller) checkAndLog(ctx context.Context) {
	if err := p.check(ctx); err != nil {
		p.logger.Warn("automation check failed", "error", sanitizeError(err))
	}
}

func (p *Poller) check(ctx context.Context) error {
	settings, err := p.store.GetAutomationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	if settings.InstagramUsername == "" || settings.DiscordWebhookURL == "" {
		return p.store.UpdateAutomationRun(ctx, time.Now(), time.Time{}, "Automation needs Instagram and Discord.", "")
	}
	webhookURL, err := secret.OpenString(p.secretKey, settings.DiscordWebhookURL)
	if err != nil {
		_ = p.store.UpdateAutomationRun(ctx, time.Now(), time.Time{}, "Discord webhook could not be read.", sanitizeError(err))
		return err
	}

	now := time.Now()
	media, err := p.profiles.FetchRecentMedia(ctx, settings.InstagramUsername, defaultFetchLimit)
	if err != nil {
		_ = p.store.UpdateAutomationRun(ctx, now, time.Time{}, "Instagram check failed.", sanitizeError(err))
		return err
	}

	seenCount, err := p.store.InstagramSeenCount(ctx, settings.InstagramUsername)
	if err != nil {
		return err
	}
	if seenCount == 0 {
		for _, item := range media {
			if err := p.store.MarkInstagramMediaSeen(ctx, seenMedia(settings.InstagramUsername, item, now, time.Time{}, true)); err != nil {
				return err
			}
		}
		return p.store.UpdateAutomationRun(ctx, now, time.Time{}, fmt.Sprintf("Watching @%s. Future posts will be delivered.", settings.InstagramUsername), "")
	}

	posted := 0
	var postedAt time.Time
	for i := len(media) - 1; i >= 0; i-- {
		if posted >= maxPostsPerPoll {
			break
		}
		item := media[i]
		seen, err := p.store.IsInstagramMediaSeen(ctx, settings.InstagramUsername, item.Ref.Shortcode)
		if err != nil {
			return err
		}
		if seen {
			continue
		}

		fixedURL := p.publicBaseURL + item.Ref.CanonicalPath()
		if err := p.discord.SendWebhook(ctx, webhookURL, fixedURL); err != nil {
			errText := sanitizeError(err)
			_ = p.store.RecordDeliveryAttempt(ctx, settings.InstagramUsername, item.Ref.Shortcode, item.Ref.Type, "error", errText, now)
			_ = p.store.UpdateAutomationRun(ctx, now, time.Time{}, "Discord delivery failed.", errText)
			return err
		}

		postedAt = time.Now()
		if err := p.store.MarkInstagramMediaSeen(ctx, seenMedia(settings.InstagramUsername, item, now, postedAt, false)); err != nil {
			return err
		}
		if err := p.store.RecordDeliveryAttempt(ctx, settings.InstagramUsername, item.Ref.Shortcode, item.Ref.Type, "ok", "", postedAt); err != nil {
			return err
		}
		posted++
	}

	status := "No new Instagram posts."
	if posted == 1 {
		status = "Posted 1 Instagram link."
	} else if posted > 1 {
		status = fmt.Sprintf("Posted %d Instagram links.", posted)
	}
	return p.store.UpdateAutomationRun(ctx, now, postedAt, status, "")
}

func seenMedia(username string, item instagram.RecentMedia, firstSeenAt, postedAt time.Time, skipped bool) cache.SeenMedia {
	instagramURL := item.InstagramURL
	if instagramURL == "" {
		instagramURL = item.Ref.OriginalURL()
	}
	return cache.SeenMedia{
		Username:       username,
		Shortcode:      item.Ref.Shortcode,
		MediaType:      item.Ref.Type,
		InstagramURL:   instagramURL,
		TakenAt:        item.TakenAt,
		FirstSeenAt:    firstSeenAt,
		PostedAt:       postedAt,
		SkippedInitial: skipped,
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 180 {
		return value[:180]
	}
	return value
}
