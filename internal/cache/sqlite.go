package cache

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"Loonstagram/internal/instagram"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

type Store struct {
	db *sql.DB
}

type Diagnostics struct {
	Posts             int                 `json:"posts"`
	OKPosts           int                 `json:"okPosts"`
	OKPostsWithMedia  int                 `json:"okPostsWithMedia"`
	ExpiredOKPosts    int                 `json:"expiredOkPosts"`
	ExpiredNonOKPosts int                 `json:"expiredNonOkPosts"`
	SeenMedia         int                 `json:"seenMedia"`
	DeliveryAttempts  int                 `json:"deliveryAttempts"`
	PostsByStatus     []StatusDiagnostics `json:"postsByStatus"`
}

type StatusDiagnostics struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, ref instagram.Ref, now time.Time) (*instagram.Post, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT original_url, username, caption, media_json, status, error, fetched_at, expires_at
FROM posts
WHERE shortcode = ? AND media_type = ? AND expires_at > ?
`, ref.Shortcode, ref.Type, now.Unix())

	return scanPost(row, ref)
}

func (s *Store) GetAny(ctx context.Context, ref instagram.Ref) (*instagram.Post, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT original_url, username, caption, media_json, status, error, fetched_at, expires_at
FROM posts
WHERE shortcode = ? AND media_type = ?
`, ref.Shortcode, ref.Type)

	return scanPost(row, ref)
}

func scanPost(row *sql.Row, ref instagram.Ref) (*instagram.Post, bool, error) {
	var originalURL, username, caption, mediaJSON, status, errText string
	var fetchedAt, expiresAt int64
	if err := row.Scan(&originalURL, &username, &caption, &mediaJSON, &status, &errText, &fetchedAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read cache: %w", err)
	}

	var media []instagram.MediaItem
	if mediaJSON != "" {
		if err := json.Unmarshal([]byte(mediaJSON), &media); err != nil {
			return nil, false, fmt.Errorf("decode cached media: %w", err)
		}
	}

	return &instagram.Post{
		Ref:         ref,
		OriginalURL: originalURL,
		Username:    username,
		Caption:     caption,
		Media:       media,
		Status:      status,
		Error:       errText,
		FetchedAt:   time.Unix(fetchedAt, 0),
		ExpiresAt:   time.Unix(expiresAt, 0),
	}, true, nil
}

func (s *Store) ListGalleryPosts(ctx context.Context, username string, limit int, _ time.Time) ([]instagram.Post, error) {
	if limit <= 0 || limit > 120 {
		limit = 30
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT posts.shortcode, posts.media_type, posts.original_url, posts.username, posts.caption,
  posts.media_json, posts.status, posts.error, posts.fetched_at, posts.expires_at
FROM posts
LEFT JOIN instagram_seen_media seen
  ON lower(seen.username) = lower(posts.username)
  AND seen.shortcode = posts.shortcode
WHERE posts.status = 'ok'
  AND (? = '' OR lower(posts.username) = lower(?))
ORDER BY COALESCE(NULLIF(seen.taken_at, 0), posts.fetched_at) DESC, posts.fetched_at DESC
LIMIT ?
`, username, username, limit)
	if err != nil {
		return nil, fmt.Errorf("list gallery posts: %w", err)
	}
	defer rows.Close()

	posts := make([]instagram.Post, 0, limit)
	for rows.Next() {
		var post instagram.Post
		var mediaJSON string
		var fetchedAt, expiresAt int64
		if err := rows.Scan(
			&post.Ref.Shortcode,
			&post.Ref.Type,
			&post.OriginalURL,
			&post.Username,
			&post.Caption,
			&mediaJSON,
			&post.Status,
			&post.Error,
			&fetchedAt,
			&expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan gallery post: %w", err)
		}
		if mediaJSON != "" {
			if err := json.Unmarshal([]byte(mediaJSON), &post.Media); err != nil {
				return nil, fmt.Errorf("decode gallery media: %w", err)
			}
		}
		post.FetchedAt = time.Unix(fetchedAt, 0)
		post.ExpiresAt = time.Unix(expiresAt, 0)
		if len(post.Media) == 0 {
			continue
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gallery posts: %w", err)
	}
	return posts, nil
}

func (s *Store) Put(ctx context.Context, post *instagram.Post) error {
	if post == nil {
		return errors.New("post is nil")
	}
	mediaJSON, err := json.Marshal(post.Media)
	if err != nil {
		return fmt.Errorf("encode media: %w", err)
	}
	if post.OriginalURL == "" {
		post.OriginalURL = post.Ref.OriginalURL()
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO posts (
  shortcode, media_type, original_url, username, caption, media_json, status, error, fetched_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(shortcode, media_type) DO UPDATE SET
  original_url = excluded.original_url,
  username = excluded.username,
  caption = excluded.caption,
  media_json = excluded.media_json,
  status = excluded.status,
  error = excluded.error,
  fetched_at = excluded.fetched_at,
  expires_at = excluded.expires_at
WHERE posts.status <> 'ok' OR excluded.status = 'ok'
`, post.Ref.Shortcode, post.Ref.Type, post.OriginalURL, post.Username, post.Caption, string(mediaJSON), post.Status, post.Error, post.FetchedAt.Unix(), post.ExpiresAt.Unix())
	if err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, ref instagram.Ref) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM posts WHERE shortcode = ? AND media_type = ?`, ref.Shortcode, ref.Type)
	if err != nil {
		return 0, fmt.Errorf("delete cache: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return count, nil
}

func (s *Store) Diagnostics(ctx context.Context, now time.Time) (Diagnostics, error) {
	var out Diagnostics
	if err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'ok' AND media_json <> '' AND media_json <> '[]' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status = 'ok' AND expires_at < ? THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status <> 'ok' AND expires_at < ? THEN 1 ELSE 0 END), 0)
FROM posts
`, now.Unix(), now.Unix()).Scan(&out.Posts, &out.OKPosts, &out.OKPostsWithMedia, &out.ExpiredOKPosts, &out.ExpiredNonOKPosts); err != nil {
		return out, fmt.Errorf("read post diagnostics: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instagram_seen_media`).Scan(&out.SeenMedia); err != nil {
		return out, fmt.Errorf("read seen media diagnostics: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM delivery_attempts`).Scan(&out.DeliveryAttempts); err != nil {
		return out, fmt.Errorf("read delivery diagnostics: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT status, COUNT(*)
FROM posts
GROUP BY status
ORDER BY COUNT(*) DESC, status
`)
	if err != nil {
		return out, fmt.Errorf("read status diagnostics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item StatusDiagnostics
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			return out, fmt.Errorf("scan status diagnostics: %w", err)
		}
		out.PostsByStatus = append(out.PostsByStatus, item)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read status diagnostics rows: %w", err)
	}
	return out, nil
}

func (s *Store) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM posts WHERE status <> 'ok' AND expires_at < ?`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("cleanup cache: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return count, nil
}

func (s *Store) StartCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.Cleanup(ctx, time.Now())
			if err != nil {
				logger.Warn("cache cleanup failed", "error", err)
				continue
			}
			if count > 0 {
				logger.Debug("cache cleanup complete", "deleted", count)
			}
		}
	}
}
