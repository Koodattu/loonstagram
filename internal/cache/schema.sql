CREATE TABLE IF NOT EXISTS posts (
  shortcode TEXT NOT NULL,
  media_type TEXT NOT NULL,
  original_url TEXT NOT NULL,
  username TEXT,
  caption TEXT,
  media_json TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT,
  fetched_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY (shortcode, media_type)
);

CREATE INDEX IF NOT EXISTS idx_posts_expires_at ON posts (expires_at);

CREATE TABLE IF NOT EXISTS automation_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  instagram_username TEXT NOT NULL DEFAULT '',
  discord_webhook_url TEXT NOT NULL DEFAULT '',
  discord_webhook_id TEXT NOT NULL DEFAULT '',
  discord_webhook_name TEXT NOT NULL DEFAULT '',
  discord_channel_id TEXT NOT NULL DEFAULT '',
  discord_channel_name TEXT NOT NULL DEFAULT '',
  discord_guild_id TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 0,
  last_checked_at INTEGER NOT NULL DEFAULT 0,
  last_posted_at INTEGER NOT NULL DEFAULT 0,
  last_status TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS instagram_seen_media (
  username TEXT NOT NULL,
  shortcode TEXT NOT NULL,
  media_type TEXT NOT NULL,
  instagram_url TEXT NOT NULL,
  taken_at INTEGER NOT NULL DEFAULT 0,
  first_seen_at INTEGER NOT NULL,
  posted_at INTEGER NOT NULL DEFAULT 0,
  skipped_initial INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (username, shortcode)
);

CREATE INDEX IF NOT EXISTS idx_instagram_seen_media_username_seen
ON instagram_seen_media (username, first_seen_at);

CREATE TABLE IF NOT EXISTS delivery_attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL,
  shortcode TEXT NOT NULL,
  media_type TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_delivery_attempts_created_at
ON delivery_attempts (created_at);

CREATE TABLE IF NOT EXISTS discord_oauth_states (
  state TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_discord_oauth_states_expires_at
ON discord_oauth_states (expires_at);
