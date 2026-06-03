package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const defaultInstagramWebAppID = "936619743392459"

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)

type ProfileFetchError struct {
	Kind    string
	Message string
}

func (e ProfileFetchError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Kind
}

type ProfileClientConfig struct {
	Timeout      time.Duration
	MaxBodyBytes int64
	AppID        string
	SessionID    string
}

type ProfileClient struct {
	httpClient   *http.Client
	maxBodyBytes int64
	appID        string
	sessionID    string
}

type RecentMedia struct {
	Ref          Ref
	Username     string
	InstagramURL string
	Caption      string
	TakenAt      time.Time
}

func NewProfileClient(cfg ProfileClientConfig) *ProfileClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	maxBodyBytes := cfg.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 2 * 1024 * 1024
	}
	appID := strings.TrimSpace(cfg.AppID)
	if appID == "" {
		appID = defaultInstagramWebAppID
	}
	return &ProfileClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxBodyBytes: maxBodyBytes,
		appID:        appID,
		sessionID:    strings.TrimSpace(cfg.SessionID),
	}
}

func ValidUsername(username string) bool {
	return usernamePattern.MatchString(username)
}

func NormalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(strings.TrimPrefix(username, "@"))
	if !ValidUsername(username) {
		return "", ErrUnsupportedURL
	}
	return username, nil
}

func (c *ProfileClient) FetchRecentMedia(ctx context.Context, username string, limit int) ([]RecentMedia, error) {
	username, err := NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 24 {
		limit = 12
	}

	values := url.Values{}
	values.Set("username", username)
	endpoint := "https://i.instagram.com/api/v1/users/web_profile_info/?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ProfileFetchError{Kind: FetchErrorNetwork, Message: err.Error()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Loonstagram/1.0; +https://github.com/)")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-IG-App-ID", c.appID)
	if c.sessionID != "" {
		req.Header.Set("Cookie", "sessionid="+c.sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ProfileFetchError{Kind: FetchErrorNetwork, Message: sanitizeError(err)}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ProfileFetchError{Kind: FetchErrorNotFound, Message: "instagram profile not found"}
	case http.StatusTooManyRequests, http.StatusForbidden, http.StatusUnauthorized:
		return nil, ProfileFetchError{Kind: FetchErrorBlocked, Message: "instagram profile fetch blocked"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ProfileFetchError{Kind: FetchErrorNetwork, Message: fmt.Sprintf("instagram returned status %d", resp.StatusCode)}
	}

	body, err := readLimited(resp.Body, c.maxBodyBytes)
	if err != nil {
		return nil, ProfileFetchError{Kind: FetchErrorNetwork, Message: err.Error()}
	}

	media, err := ParseProfileMediaJSON(username, body, limit)
	if err != nil {
		return nil, ProfileFetchError{Kind: FetchErrorParse, Message: err.Error()}
	}
	return media, nil
}

func ParseProfileMediaJSON(username string, body []byte, limit int) ([]RecentMedia, error) {
	if limit <= 0 {
		limit = 12
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	media := make([]RecentMedia, 0, limit)
	for _, node := range profileMediaNodes(payload) {
		item, ok := recentMediaFromProfileNode(username, node)
		if !ok {
			continue
		}
		media = append(media, item)
		if len(media) >= limit {
			return media, nil
		}
	}

	if len(media) == 0 {
		for _, item := range asSlice(payload["items"]) {
			node := asMap(item)
			if node == nil {
				continue
			}
			mediaItem, ok := recentMediaFromAPIItem(username, node)
			if !ok {
				continue
			}
			media = append(media, mediaItem)
			if len(media) >= limit {
				return media, nil
			}
		}
	}

	if len(media) == 0 {
		return nil, errors.New("no recent instagram media found")
	}
	return media, nil
}

func profileMediaNodes(payload map[string]any) []map[string]any {
	roots := []map[string]any{
		asMap(asMap(payload["data"])["user"]),
		asMap(asMap(payload["graphql"])["user"]),
		asMap(payload["user"]),
	}

	nodes := make([]map[string]any, 0, 12)
	for _, root := range roots {
		if root == nil {
			continue
		}
		timeline := asMap(root["edge_owner_to_timeline_media"])
		if timeline == nil {
			timeline = asMap(root["edge_web_feed_timeline"])
		}
		for _, edge := range asSlice(timeline["edges"]) {
			node := asMap(asMap(edge)["node"])
			if node == nil {
				continue
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func recentMediaFromProfileNode(username string, node map[string]any) (RecentMedia, bool) {
	shortcode := asString(node["shortcode"])
	if !ValidShortcode(shortcode) {
		return RecentMedia{}, false
	}
	mediaType := TypePost
	productType := strings.ToLower(firstString(asString(node["product_type"]), asString(node["media_product_type"])))
	if strings.Contains(productType, "clips") || asMap(node["clips_metadata"]) != nil {
		mediaType = TypeReel
	}

	ref, err := NewRef(mediaType, shortcode)
	if err != nil {
		return RecentMedia{}, false
	}
	return RecentMedia{
		Ref:          ref,
		Username:     username,
		InstagramURL: ref.OriginalURL(),
		Caption:      captionFromProfileNode(node),
		TakenAt:      unixProfileTime(asInt(node["taken_at_timestamp"])),
	}, true
}

func recentMediaFromAPIItem(username string, item map[string]any) (RecentMedia, bool) {
	shortcode := firstString(asString(item["code"]), asString(item["shortcode"]))
	if !ValidShortcode(shortcode) {
		return RecentMedia{}, false
	}
	mediaType := TypePost
	productType := strings.ToLower(firstString(asString(item["product_type"]), asString(item["media_product_type"])))
	if strings.Contains(productType, "clips") {
		mediaType = TypeReel
	}

	ref, err := NewRef(mediaType, shortcode)
	if err != nil {
		return RecentMedia{}, false
	}
	return RecentMedia{
		Ref:          ref,
		Username:     username,
		InstagramURL: ref.OriginalURL(),
		Caption:      captionFromAPIItem(item),
		TakenAt:      unixProfileTime(asInt(item["taken_at"])),
	}, true
}

func captionFromProfileNode(node map[string]any) string {
	captions := asMap(node["edge_media_to_caption"])
	for _, edge := range asSlice(captions["edges"]) {
		text := asString(asMap(asMap(edge)["node"])["text"])
		if text != "" {
			return CleanCaption(text)
		}
	}
	return ""
}

func captionFromAPIItem(item map[string]any) string {
	if caption := asMap(item["caption"]); caption != nil {
		return CleanCaption(asString(caption["text"]))
	}
	return ""
}

func unixProfileTime(value int) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(value), 0)
}
