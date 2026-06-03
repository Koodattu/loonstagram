package instagram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DebugReport struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Ref         Ref          `json:"ref"`
	Fetches     []DebugFetch `json:"fetches"`
}

type DebugFetch struct {
	Name            string              `json:"name"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	RequestHeaders  map[string][]string `json:"requestHeaders,omitempty"`
	Status          string              `json:"status,omitempty"`
	StatusCode      int                 `json:"statusCode,omitempty"`
	ResponseHeaders map[string][]string `json:"responseHeaders,omitempty"`
	DurationMS      int64               `json:"durationMs"`
	BodyBytes       int                 `json:"bodyBytes"`
	BodyTruncated   bool                `json:"bodyTruncated"`
	Body            string              `json:"body,omitempty"`
	ExtractedJSON   []DebugJSONBlock    `json:"extractedJson,omitempty"`
	ParsedPost      *Post               `json:"parsedPost,omitempty"`
	ParseError      string              `json:"parseError,omitempty"`
	Error           string              `json:"error,omitempty"`
}

type DebugJSONBlock struct {
	Key    string `json:"key"`
	Index  int    `json:"index"`
	Bytes  int    `json:"bytes"`
	Raw    string `json:"raw"`
	Pretty string `json:"pretty,omitempty"`
}

func (c *Client) DebugFetchPost(ctx context.Context, ref Ref) DebugReport {
	report := DebugReport{
		GeneratedAt: time.Now(),
		Ref:         ref,
	}
	report.Fetches = append(report.Fetches,
		c.debugFetch(ctx, ref, "embed_page", ref.EmbedURL()),
		c.debugFetch(ctx, ref, "original_page", ref.OriginalURL()),
	)
	return report
}

func (c *Client) debugFetch(ctx context.Context, ref Ref, name, target string) DebugFetch {
	out := DebugFetch{
		Name:   name,
		Method: http.MethodGet,
		URL:    target,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	setInstagramRequestHeaders(req)
	out.RequestHeaders = redactedHeaders(req.Header)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	out.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		out.Error = sanitizeError(err)
		return out
	}
	defer resp.Body.Close()

	out.Status = resp.Status
	out.StatusCode = resp.StatusCode
	out.ResponseHeaders = redactedHeaders(resp.Header)
	out.Error = debugStatusError(resp.StatusCode)

	body, truncated, err := readDebugLimited(resp.Body, c.maxBodyBytes)
	out.BodyBytes = len(body)
	out.BodyTruncated = truncated
	out.Body = string(body)
	if err != nil {
		out.Error = combineDebugErrors(out.Error, err.Error())
	}

	out.ExtractedJSON = extractDebugJSON(out.Body)
	post, parseErr := ParseEmbedHTML(ref, out.Body)
	if parseErr != nil {
		out.ParseError = parseErr.Error()
		return out
	}
	post.Status = "ok"
	post.Error = ""
	if post.OriginalURL == "" {
		post.OriginalURL = ref.OriginalURL()
	}
	out.ParsedPost = post
	return out
}

func combineDebugErrors(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "; ")
}

func debugStatusError(status int) string {
	switch {
	case status == http.StatusNotFound:
		return "instagram metadata not found"
	case status == http.StatusTooManyRequests || status == http.StatusForbidden:
		return "instagram metadata fetch blocked"
	case status < 200 || status >= 300:
		return fmt.Sprintf("instagram returned status %d", status)
	default:
		return ""
	}
}

func setInstagramRequestHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; InstaFix/1.0; +https://github.com/)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
}

func readDebugLimited(reader io.Reader, limit int64) ([]byte, bool, error) {
	if limit <= 0 {
		limit = 2 * 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, sanitizeFetchError(err)
	}
	if int64(len(body)) > limit {
		return body[:limit], true, fmt.Errorf("instagram response exceeded maximum size; showing first %d bytes", limit)
	}
	return body, false, nil
}

func extractDebugJSON(body string) []DebugJSONBlock {
	out := make([]DebugJSONBlock, 0)
	for _, key := range []string{"shortcode_media", "xdt_shortcode_media", "items"} {
		for index, raw := range extractJSONValuesAfterKey(body, key, 8) {
			out = append(out, DebugJSONBlock{
				Key:    key,
				Index:  index + 1,
				Bytes:  len(raw),
				Raw:    raw,
				Pretty: prettyJSON(raw),
			})
		}
	}
	return out
}

func prettyJSON(raw string) string {
	if decoded, ok := decodeEscapedJSONValue(raw); ok {
		raw = decoded
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return ""
	}
	return buf.String()
}

func redactedHeaders(headers http.Header) map[string][]string {
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		if sensitiveHeader(key) {
			out[key] = []string{"[redacted]"}
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	return out
}

func sensitiveHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "cookie", "proxy-authorization", "set-cookie", "x-csrftoken":
		return true
	default:
		return false
	}
}
