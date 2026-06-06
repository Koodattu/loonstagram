package instagram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var debugImageURLPattern = regexp.MustCompile(`https?(?::|\\u003a)[/\\]+scontent\.cdninstagram\.com[^"'<>\s]+`)

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

type DebugMediaCandidate struct {
	Source  string `json:"source"`
	URL     string `json:"url"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Cropped bool   `json:"cropped"`
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Loonstagram/1.0; +https://github.com/)")
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

func ExtractDebugMediaCandidates(report DebugReport) []DebugMediaCandidate {
	out := make([]DebugMediaCandidate, 0)
	seen := make(map[string]bool)
	for _, fetch := range report.Fetches {
		for _, raw := range debugImageURLPattern.FindAllString(fetch.Body, -1) {
			appendDebugMediaCandidate(fetch.Name+" body", normalizeDebugMediaURL(raw), 0, 0, seen, &out)
		}
		for _, block := range fetch.ExtractedJSON {
			raw := block.Raw
			if decoded, ok := decodeEscapedJSONValue(raw); ok {
				raw = decoded
			}
			var value any
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				continue
			}
			source := fmt.Sprintf("%s %s #%d", fetch.Name, block.Key, block.Index)
			appendDebugMediaCandidates(source, value, seen, &out)
		}
	}
	return out
}

func appendDebugMediaCandidates(source string, value any, seen map[string]bool, out *[]DebugMediaCandidate) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			appendDebugMediaCandidates(source, item, seen, out)
		}
	case map[string]any:
		width := firstPositiveInt(
			asInt(typed["width"]),
			asInt(typed["config_width"]),
			asInt(typed["original_width"]),
		)
		height := firstPositiveInt(
			asInt(typed["height"]),
			asInt(typed["config_height"]),
			asInt(typed["original_height"]),
		)
		for _, key := range []string{"url", "src", "display_url", "display_uri", "thumbnail_src", "thumbnail_url"} {
			appendDebugMediaCandidate(source, asString(typed[key]), width, height, seen, out)
		}
		for _, child := range typed {
			appendDebugMediaCandidates(source, child, seen, out)
		}
	}
}

func normalizeDebugMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, `\u003a`, ":")
	raw = strings.ReplaceAll(raw, `\/`, "/")
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	return html.UnescapeString(raw)
}

func appendDebugMediaCandidate(source, raw string, width, height int, seen map[string]bool, out *[]DebugMediaCandidate) {
	raw = normalizeDebugMediaURL(raw)
	if !looksLikeImageURL(raw) || seen[raw] {
		return
	}
	seen[raw] = true
	*out = append(*out, DebugMediaCandidate{
		Source:  source,
		URL:     raw,
		Width:   width,
		Height:  height,
		Cropped: LooksCroppedMediaURL(raw),
	})
}

func looksLikeImageURL(raw string) bool {
	if !strings.Contains(raw, "cdninstagram.com/") {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp")
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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
