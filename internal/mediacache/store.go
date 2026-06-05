package mediacache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	root     string
	maxBytes int64
}

type Entry struct {
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
}

const defaultContentType = "application/octet-stream"

func Open(root string, maxBytes int64) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("media cache path is required")
	}
	if maxBytes <= 0 {
		return nil, errors.New("media cache max bytes must be positive")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create media cache directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		return nil, fmt.Errorf("create media cache temp directory: %w", err)
	}
	return &Store{root: root, maxBytes: maxBytes}, nil
}

func (s *Store) Open(key string) (*os.File, Entry, bool, error) {
	var zero Entry
	if s == nil {
		return nil, zero, false, nil
	}
	bodyPath, metaPath, err := s.paths(key)
	if err != nil {
		return nil, zero, false, err
	}

	meta, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, zero, false, nil
		}
		return nil, zero, false, fmt.Errorf("read media metadata: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(meta, &entry); err != nil {
		return nil, zero, false, fmt.Errorf("decode media metadata: %w", err)
	}
	file, err := os.Open(bodyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, zero, false, nil
		}
		return nil, zero, false, fmt.Errorf("open cached media: %w", err)
	}
	return file, entry, true, nil
}

func (s *Store) Put(ctx context.Context, key, contentType string, body io.Reader) (Entry, error) {
	var zero Entry
	if s == nil {
		return zero, errors.New("media cache is nil")
	}
	bodyPath, metaPath, err := s.paths(key)
	if err != nil {
		return zero, err
	}
	if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err != nil {
		return zero, fmt.Errorf("create media cache shard: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "media-*")
	if err != nil {
		return zero, fmt.Errorf("create media cache temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	limited := io.LimitReader(&contextReader{ctx: ctx, r: body}, s.maxBytes+1)
	size, err := io.Copy(tmp, limited)
	closeErr := tmp.Close()
	if err != nil {
		return zero, fmt.Errorf("write media cache body: %w", err)
	}
	if closeErr != nil {
		return zero, fmt.Errorf("close media cache body: %w", closeErr)
	}
	if size > s.maxBytes {
		return zero, fmt.Errorf("media body exceeds %d bytes", s.maxBytes)
	}

	entry := Entry{
		ContentType: firstString(contentType, defaultContentType),
		Size:        size,
		CreatedAt:   time.Now().UTC(),
	}
	metaBody, err := json.Marshal(entry)
	if err != nil {
		return zero, fmt.Errorf("encode media metadata: %w", err)
	}

	metaTmp, err := os.CreateTemp(filepath.Join(s.root, "tmp"), "media-meta-*")
	if err != nil {
		return zero, fmt.Errorf("create media metadata temp file: %w", err)
	}
	metaTmpPath := metaTmp.Name()
	removeMetaTmp := true
	defer func() {
		if removeMetaTmp {
			_ = os.Remove(metaTmpPath)
		}
	}()
	if _, err := metaTmp.Write(metaBody); err != nil {
		_ = metaTmp.Close()
		return zero, fmt.Errorf("write media metadata: %w", err)
	}
	if err := metaTmp.Close(); err != nil {
		return zero, fmt.Errorf("close media metadata: %w", err)
	}

	if err := replaceFile(tmpPath, bodyPath); err != nil {
		return zero, fmt.Errorf("commit media cache body: %w", err)
	}
	removeTmp = false
	if err := replaceFile(metaTmpPath, metaPath); err != nil {
		_ = os.Remove(bodyPath)
		return zero, fmt.Errorf("commit media metadata: %w", err)
	}
	removeMetaTmp = false
	return entry, nil
}

func replaceFile(source, target string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}

func (s *Store) paths(key string) (string, string, error) {
	if !validKey(key) {
		return "", "", fmt.Errorf("invalid media cache key %q", key)
	}
	shard := key
	if len(key) >= 2 {
		shard = key[:2]
	}
	bodyPath := filepath.Join(s.root, shard, key+".bin")
	metaPath := filepath.Join(s.root, shard, key+".json")
	return bodyPath, metaPath, nil
}

func validKey(key string) bool {
	if key == "" || len(key) > 160 {
		return false
	}
	for _, r := range key {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
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
