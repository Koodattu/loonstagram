package automation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/discord"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/secret"
)

const (
	defaultPollInterval = 2 * time.Minute
	defaultFetchLimit   = 24
	maxPostsPerPoll     = 5
)

type ProfileFetcher interface {
	FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error)
}

type PostFetcher interface {
	FetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error)
}

type DiscordSender interface {
	SendWebhook(ctx context.Context, webhookURL, content string) error
}

type Poller struct {
	store         *cache.Store
	profiles      ProfileFetcher
	posts         PostFetcher
	discord       DiscordSender
	publicBaseURL string
	secretKey     string
	interval      time.Duration
	cacheTTL      time.Duration
	logger        *slog.Logger
}

type Options struct {
	Store         *cache.Store
	Profiles      ProfileFetcher
	Posts         PostFetcher
	Discord       DiscordSender
	PublicBaseURL string
	SecretKey     string
	Interval      time.Duration
	CacheTTL      time.Duration
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
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 6 * time.Hour
	}
	return &Poller{
		store:         opts.Store,
		profiles:      opts.Profiles,
		posts:         opts.Posts,
		discord:       opts.Discord,
		publicBaseURL: strings.TrimRight(opts.PublicBaseURL, "/"),
		secretKey:     opts.SecretKey,
		interval:      interval,
		cacheTTL:      cacheTTL,
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
	if settings.InstagramUsername == "" {
		return p.store.UpdateAutomationRun(ctx, time.Now(), time.Time{}, "Automation needs an Instagram profile.", "")
	}
	webhookURL := ""
	if settings.DiscordWebhookURL != "" {
		webhookURL, err = secret.OpenString(p.secretKey, settings.DiscordWebhookURL)
		if err != nil {
			_ = p.store.UpdateAutomationRun(ctx, time.Now(), time.Time{}, "Discord webhook could not be read.", sanitizeError(err))
			return err
		}
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
			if err := p.ensurePostCached(ctx, item, now); err != nil {
				p.logger.Warn("initial post cache failed", "shortcode", item.Ref.Shortcode, "error", sanitizeError(err))
			}
			if err := p.store.MarkInstagramMediaSeen(ctx, seenMedia(settings.InstagramUsername, item, now, time.Time{}, true)); err != nil {
				return err
			}
		}
		status := fmt.Sprintf("Watching @%s. Recent posts cached.", settings.InstagramUsername)
		if webhookURL != "" {
			status = fmt.Sprintf("Watching @%s. Future posts will be delivered.", settings.InstagramUsername)
		}
		return p.store.UpdateAutomationRun(ctx, now, time.Time{}, status, "")
	}

	posted := 0
	cachedOnly := 0
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

		if err := p.ensurePostCached(ctx, item, now); err != nil {
			p.logger.Warn("new post cache failed", "shortcode", item.Ref.Shortcode, "error", sanitizeError(err))
		}

		if webhookURL == "" {
			if err := p.store.MarkInstagramMediaSeen(ctx, seenMedia(settings.InstagramUsername, item, now, time.Time{}, false)); err != nil {
				return err
			}
			cachedOnly++
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
	} else if cachedOnly == 1 {
		status = "Cached 1 new Instagram post. Discord is not connected."
	} else if cachedOnly > 1 {
		status = fmt.Sprintf("Cached %d new Instagram posts. Discord is not connected.", cachedOnly)
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

func (p *Poller) ensurePostCached(ctx context.Context, item instagram.RecentMedia, now time.Time) error {
	if p.posts == nil {
		return nil
	}
	if _, ok, err := p.store.Get(ctx, item.Ref, now); err != nil {
		return err
	} else if ok {
		return nil
	}

	post, err := p.posts.FetchPost(ctx, item.Ref)
	if err != nil {
		return err
	}
	if post == nil {
		return nil
	}
	post.Ref = item.Ref
	post.Status = "ok"
	post.Error = ""
	if post.OriginalURL == "" {
		post.OriginalURL = firstString(item.InstagramURL, item.Ref.OriginalURL())
	}
	if post.Username == "" {
		post.Username = item.Username
	}
	if post.Caption == "" {
		post.Caption = item.Caption
	}
	post.FetchedAt = now
	post.ExpiresAt = now.Add(p.cacheTTL)
	return p.store.Put(ctx, post)
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

func firstString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
