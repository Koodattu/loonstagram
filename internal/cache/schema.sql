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
