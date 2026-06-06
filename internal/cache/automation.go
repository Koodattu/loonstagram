package cache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AutomationSettings struct {
	InstagramUsername   string
	DiscordWebhookURL   string
	DiscordWebhookID    string
	DiscordWebhookName  string
	DiscordChannelID    string
	DiscordChannelName  string
	DiscordGuildID      string
	Enabled             bool
	PollIntervalMinutes int
	LastCheckedAt       time.Time
	LastPostedAt        time.Time
	LastStatus          string
	LastError           string
	UpdatedAt           time.Time
}

type DiscordWebhook struct {
	URL         string
	ID          string
	Name        string
	ChannelID   string
	ChannelName string
	GuildID     string
}

type SeenMedia struct {
	Username       string
	Shortcode      string
	MediaType      string
	InstagramURL   string
	TakenAt        time.Time
	FirstSeenAt    time.Time
	PostedAt       time.Time
	SkippedInitial bool
}

func (s *Store) GetAutomationSettings(ctx context.Context) (AutomationSettings, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT instagram_username, discord_webhook_url, discord_webhook_id, discord_webhook_name,
  discord_channel_id, discord_channel_name, discord_guild_id, enabled, poll_interval_minutes, last_checked_at,
  last_posted_at, last_status, last_error, updated_at
FROM automation_settings
WHERE id = 1
`)

	var settings AutomationSettings
	var enabled int
	var lastCheckedAt, lastPostedAt, updatedAt int64
	if err := row.Scan(
		&settings.InstagramUsername,
		&settings.DiscordWebhookURL,
		&settings.DiscordWebhookID,
		&settings.DiscordWebhookName,
		&settings.DiscordChannelID,
		&settings.DiscordChannelName,
		&settings.DiscordGuildID,
		&enabled,
		&settings.PollIntervalMinutes,
		&lastCheckedAt,
		&lastPostedAt,
		&settings.LastStatus,
		&settings.LastError,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AutomationSettings{}, nil
		}
		return AutomationSettings{}, fmt.Errorf("read automation settings: %w", err)
	}

	settings.Enabled = enabled == 1
	if settings.PollIntervalMinutes <= 0 {
		settings.PollIntervalMinutes = 30
	}
	settings.LastCheckedAt = unixTime(lastCheckedAt)
	settings.LastPostedAt = unixTime(lastPostedAt)
	settings.UpdatedAt = unixTime(updatedAt)
	return settings, nil
}

func (s *Store) SaveAutomationConfig(ctx context.Context, username string, enabled bool, pollIntervalMinutes int, now time.Time) error {
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	if pollIntervalMinutes <= 0 {
		pollIntervalMinutes = 30
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO automation_settings (id, instagram_username, enabled, poll_interval_minutes, updated_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  instagram_username = excluded.instagram_username,
  enabled = excluded.enabled,
  poll_interval_minutes = excluded.poll_interval_minutes,
  last_error = '',
  updated_at = excluded.updated_at
`, username, enabledValue, pollIntervalMinutes, now.Unix())
	if err != nil {
		return fmt.Errorf("write automation config: %w", err)
	}
	return nil
}

func (s *Store) SetDiscordWebhook(ctx context.Context, webhook DiscordWebhook, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO automation_settings (
  id, discord_webhook_url, discord_webhook_id, discord_webhook_name,
  discord_channel_id, discord_channel_name, discord_guild_id, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  discord_webhook_url = excluded.discord_webhook_url,
  discord_webhook_id = excluded.discord_webhook_id,
  discord_webhook_name = excluded.discord_webhook_name,
  discord_channel_id = excluded.discord_channel_id,
  discord_channel_name = excluded.discord_channel_name,
  discord_guild_id = excluded.discord_guild_id,
  last_error = '',
  updated_at = excluded.updated_at
`, webhook.URL, webhook.ID, webhook.Name, webhook.ChannelID, webhook.ChannelName, webhook.GuildID, now.Unix())
	if err != nil {
		return fmt.Errorf("write discord webhook: %w", err)
	}
	return nil
}

func (s *Store) ClearDiscordWebhook(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO automation_settings (id, updated_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET
  discord_webhook_url = '',
  discord_webhook_id = '',
  discord_webhook_name = '',
  discord_channel_id = '',
  discord_channel_name = '',
  discord_guild_id = '',
  enabled = 0,
  last_status = '',
  last_error = '',
  updated_at = excluded.updated_at
`, now.Unix())
	if err != nil {
		return fmt.Errorf("clear discord webhook: %w", err)
	}
	return nil
}

func (s *Store) UpdateAutomationRun(ctx context.Context, checkedAt, postedAt time.Time, status, errText string) error {
	checkedUnix := checkedAt.Unix()
	postedUnix := int64(0)
	if !postedAt.IsZero() {
		postedUnix = postedAt.Unix()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO automation_settings (id, last_checked_at, last_posted_at, last_status, last_error, updated_at)
VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  last_checked_at = excluded.last_checked_at,
  last_posted_at = CASE
    WHEN excluded.last_posted_at > 0 THEN excluded.last_posted_at
    ELSE automation_settings.last_posted_at
  END,
  last_status = excluded.last_status,
  last_error = excluded.last_error,
  updated_at = excluded.updated_at
`, checkedUnix, postedUnix, status, errText, checkedUnix)
	if err != nil {
		return fmt.Errorf("write automation run: %w", err)
	}
	return nil
}

func (s *Store) InstagramSeenCount(ctx context.Context, username string) (int, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM instagram_seen_media
WHERE username = ?
`, username)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count seen instagram media: %w", err)
	}
	return count, nil
}

func (s *Store) IsInstagramMediaSeen(ctx context.Context, username, shortcode string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT 1
FROM instagram_seen_media
WHERE username = ? AND shortcode = ?
`, username, shortcode)
	var value int
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read seen instagram media: %w", err)
	}
	return true, nil
}

func (s *Store) MarkInstagramMediaSeen(ctx context.Context, media SeenMedia) error {
	skipped := 0
	if media.SkippedInitial {
		skipped = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO instagram_seen_media (
  username, shortcode, media_type, instagram_url, taken_at, first_seen_at, posted_at, skipped_initial
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(username, shortcode) DO NOTHING
`, media.Username, media.Shortcode, media.MediaType, media.InstagramURL, unixOrZero(media.TakenAt), media.FirstSeenAt.Unix(), unixOrZero(media.PostedAt), skipped)
	if err != nil {
		return fmt.Errorf("write seen instagram media: %w", err)
	}
	return nil
}

func (s *Store) RecordDeliveryAttempt(ctx context.Context, username, shortcode, mediaType, status, errText string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO delivery_attempts (username, shortcode, media_type, status, error, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, username, shortcode, mediaType, status, errText, now.Unix())
	if err != nil {
		return fmt.Errorf("write delivery attempt: %w", err)
	}
	return nil
}

func (s *Store) CreateDiscordOAuthState(ctx context.Context, state string, now, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO discord_oauth_states (state, created_at, expires_at)
VALUES (?, ?, ?)
`, state, now.Unix(), expiresAt.Unix())
	if err != nil {
		return fmt.Errorf("write discord oauth state: %w", err)
	}
	return nil
}

func (s *Store) ConsumeDiscordOAuthState(ctx context.Context, state string, now time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM discord_oauth_states
WHERE state = ? AND expires_at > ?
`, state, now.Unix())
	if err != nil {
		return false, fmt.Errorf("consume discord oauth state: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, nil
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM discord_oauth_states WHERE expires_at <= ?`, now.Unix())
	return count > 0, nil
}

func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0)
}

func unixOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}
