package instagram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	FetchErrorNotFound = "not_found"
	FetchErrorBlocked  = "blocked"
	FetchErrorParse    = "parse"
	FetchErrorNetwork  = "network"
)

type FetchError struct {
	Kind    string
	Message string
}

func (e FetchError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Kind
}

type ClientConfig struct {
	Timeout      time.Duration
	MaxBodyBytes int64
}

type Client struct {
	httpClient   *http.Client
	maxBodyBytes int64
}

func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	maxBodyBytes := cfg.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 2 * 1024 * 1024
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxBodyBytes: maxBodyBytes,
	}
}

func (c *Client) FetchPost(ctx context.Context, ref Ref) (*Post, error) {
	post, err := c.fetchPostPage(ctx, ref, ref.EmbedURL())
	if err == nil {
		if croppedMediaCount(post) > 0 {
			if fallbackPost, fallbackErr := c.fetchPostPage(ctx, ref, ref.OriginalURL()); fallbackErr == nil && betterMediaPost(fallbackPost, post) {
				return fallbackPost, nil
			}
		}
		return post, nil
	}

	var fetchErr FetchError
	if errors.As(err, &fetchErr) && fetchErr.Kind == FetchErrorParse {
		if fallbackPost, fallbackErr := c.fetchPostPage(ctx, ref, ref.OriginalURL()); fallbackErr == nil {
			return fallbackPost, nil
		}
	}
	return nil, err
}

func betterMediaPost(candidate, current *Post) bool {
	if candidate == nil || len(candidate.Media) == 0 {
		return false
	}
	if current == nil || len(current.Media) == 0 {
		return true
	}
	candidateCropped := croppedMediaCount(candidate)
	currentCropped := croppedMediaCount(current)
	if candidateCropped != currentCropped {
		return candidateCropped < currentCropped
	}
	return len(candidate.Media) > len(current.Media)
}

func croppedMediaCount(post *Post) int {
	if post == nil {
		return 0
	}
	count := 0
	for _, item := range post.Media {
		if LooksCroppedMediaURL(item.URL) || LooksCroppedMediaURL(item.PosterURL) {
			count++
		}
	}
	return count
}

func (c *Client) fetchPostPage(ctx context.Context, ref Ref, target string) (*Post, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, FetchError{Kind: FetchErrorNetwork, Message: err.Error()}
	}
	setInstagramRequestHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, FetchError{Kind: FetchErrorNetwork, Message: sanitizeError(err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, FetchError{Kind: FetchErrorNotFound, Message: "instagram metadata not found"}
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return nil, FetchError{Kind: FetchErrorBlocked, Message: "instagram metadata fetch blocked"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, FetchError{Kind: FetchErrorNetwork, Message: fmt.Sprintf("instagram returned status %d", resp.StatusCode)}
	}

	body, err := readLimited(resp.Body, c.maxBodyBytes)
	if err != nil {
		return nil, FetchError{Kind: FetchErrorNetwork, Message: err.Error()}
	}

	post, err := ParseEmbedHTML(ref, string(body))
	if err != nil {
		return nil, FetchError{Kind: FetchErrorParse, Message: err.Error()}
	}
	post.Status = "ok"
	post.Error = ""
	if post.OriginalURL == "" {
		post.OriginalURL = ref.OriginalURL()
	}
	return post, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, sanitizeFetchError(err)
	}
	if int64(len(body)) > limit {
		return nil, errors.New("instagram response exceeded maximum size")
	}
	return body, nil
}

func sanitizeFetchError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(sanitizeError(err))
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 180 {
		return message[:180]
	}
	return message
}
