package instagram

import (
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	metaTagPattern      = regexp.MustCompile(`(?is)<meta\s+([^>]+)>`)
	attrPattern         = regexp.MustCompile(`(?is)([a-zA-Z_:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	metaUsernamePattern = regexp.MustCompile(`(?:^|[\s(])@([A-Za-z0-9_.]+)`)
	croppedStpPattern   = regexp.MustCompile(`(?:^|_)c\d+(?:\.\d+){3}a(?:_|$)`)
)

func ParseEmbedHTML(ref Ref, body string) (*Post, error) {
	post := &Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Media:       make([]MediaItem, 0, 1),
	}

	for _, key := range []string{"shortcode_media", "xdt_shortcode_media"} {
		for _, raw := range extractJSONValuesAfterKey(body, key, 4) {
			var node map[string]any
			if err := unmarshalJSONValue(raw, &node); err == nil {
				applyGraphQLNode(post, node)
			}
			if post.Username != "" && post.Caption != "" && len(post.Media) > 0 {
				break
			}
		}
		if post.Username != "" && post.Caption != "" && len(post.Media) > 0 {
			break
		}
	}

	if post.Username == "" || post.Caption == "" || len(post.Media) == 0 {
		applyInstagramAPIFallback(post, body)
	}

	meta := parseMetaTags(body)
	applyMetaFallback(post, meta)

	post.Caption = CleanCaption(post.Caption)
	if post.Username == "" && post.Caption == "" {
		return nil, errors.New("no usable instagram metadata found")
	}

	return post, nil
}

func applyInstagramAPIFallback(post *Post, body string) {
	for _, raw := range extractJSONValuesAfterKey(body, "items", 24) {
		var items []any
		if err := unmarshalJSONValue(raw, &items); err != nil {
			continue
		}
		if applyInstagramAPIItems(post, items) {
			return
		}
	}
}

func CleanCaption(value string) string {
	value = strings.TrimSpace(html.UnescapeString(value))
	if value == "" {
		return ""
	}

	lines := strings.Split(value, "\n")
	clean := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if !blank {
				clean = append(clean, "")
			}
			blank = true
			continue
		}
		clean = append(clean, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}

func CaptionPreview(value string, limit int) string {
	value = CleanCaption(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "..."
}

func applyGraphQLNode(post *Post, node map[string]any) {
	if owner := asMap(node["owner"]); owner != nil {
		post.Username = asString(owner["username"])
	}

	if captions := asMap(node["edge_media_to_caption"]); captions != nil {
		edges := asSlice(captions["edges"])
		if len(edges) > 0 {
			if edge := asMap(edges[0]); edge != nil {
				if captionNode := asMap(edge["node"]); captionNode != nil {
					post.Caption = asString(captionNode["text"])
				}
			}
		}
	}

	children := make([]any, 0)
	if sidecar := asMap(node["edge_sidecar_to_children"]); sidecar != nil {
		children = asSlice(sidecar["edges"])
	}
	if len(children) == 0 {
		appendMediaFromNode(post, node)
		return
	}
	for _, child := range children {
		edge := asMap(child)
		if edge == nil {
			continue
		}
		childNode := asMap(edge["node"])
		if childNode == nil {
			continue
		}
		appendMediaFromNode(post, childNode)
	}
}

func applyInstagramAPIItems(post *Post, items []any) bool {
	for _, item := range items {
		node := asMap(item)
		if node == nil || !looksLikeInstagramAPIItem(node) {
			continue
		}
		beforeUsername := post.Username
		beforeCaption := post.Caption
		beforeMedia := len(post.Media)
		applyInstagramAPIItem(post, node)
		if post.Username != beforeUsername || post.Caption != beforeCaption || len(post.Media) != beforeMedia {
			return true
		}
	}
	return false
}

func looksLikeInstagramAPIItem(node map[string]any) bool {
	return asMap(node["user"]) != nil ||
		asMap(node["caption"]) != nil ||
		asMap(node["image_versions2"]) != nil ||
		len(asSlice(node["video_versions"])) > 0 ||
		len(asSlice(node["carousel_media"])) > 0
}

func applyInstagramAPIItem(post *Post, item map[string]any) {
	if post.Username == "" {
		if user := asMap(item["user"]); user != nil {
			post.Username = asString(user["username"])
		}
	}
	if post.Caption == "" {
		if caption := asMap(item["caption"]); caption != nil {
			post.Caption = asString(caption["text"])
		}
	}

	children := asSlice(item["carousel_media"])
	if len(children) == 0 {
		appendMediaFromAPIItem(post, item)
		return
	}
	for _, child := range children {
		childNode := asMap(child)
		if childNode == nil {
			continue
		}
		appendMediaFromAPIItem(post, childNode)
	}
}

func appendMediaFromNode(post *Post, node map[string]any) {
	displayURL := largestDisplayResource(node)
	videoURL := asString(node["video_url"])
	isVideo := asBool(node["is_video"]) || videoURL != ""

	width, height := dimensions(node)
	if isVideo {
		if videoURL == "" && displayURL == "" {
			return
		}
		post.Media = append(post.Media, MediaItem{
			Kind:        "video",
			URL:         videoURL,
			PosterURL:   displayURL,
			Width:       width,
			Height:      height,
			ContentType: "video/mp4",
		})
		return
	}

	if displayURL == "" {
		return
	}
	post.Media = append(post.Media, MediaItem{
		Kind:        "image",
		URL:         displayURL,
		Width:       width,
		Height:      height,
		ContentType: "image/jpeg",
	})
}

func appendMediaFromAPIItem(post *Post, item map[string]any) {
	imageURL, imageWidth, imageHeight := bestImageVersion(item)
	videoURL, videoWidth, videoHeight := bestVideoVersion(item)
	isVideo := asInt(item["media_type"]) == 2 || videoURL != ""

	if isVideo {
		if videoURL == "" && imageURL == "" {
			return
		}
		width, height := imageWidth, imageHeight
		if width == 0 {
			width = videoWidth
		}
		if height == 0 {
			height = videoHeight
		}
		post.Media = append(post.Media, MediaItem{
			Kind:        "video",
			URL:         videoURL,
			PosterURL:   imageURL,
			Width:       width,
			Height:      height,
			ContentType: "video/mp4",
		})
		return
	}

	if imageURL == "" {
		return
	}
	post.Media = append(post.Media, MediaItem{
		Kind:        "image",
		URL:         imageURL,
		Width:       imageWidth,
		Height:      imageHeight,
		ContentType: "image/jpeg",
	})
}

func largestDisplayResource(node map[string]any) string {
	best := imageVersionCandidate{}
	for _, key := range []string{"display_url", "thumbnail_src"} {
		raw := asString(node[key])
		candidate := imageVersionCandidate{
			URL:     raw,
			Cropped: LooksCroppedMediaURL(raw),
		}
		if betterImageCandidate(candidate, best) {
			best = candidate
		}
	}
	resources := asSlice(node["display_resources"])
	for _, item := range resources {
		resource := asMap(item)
		if resource == nil {
			continue
		}
		candidate := imageVersionCandidate{
			URL:     asString(resource["src"]),
			Width:   asInt(resource["config_width"]),
			Height:  asInt(resource["config_height"]),
			Cropped: LooksCroppedMediaURL(asString(resource["src"])),
		}
		if betterImageCandidate(candidate, best) {
			best = candidate
		}
	}
	return best.URL
}

func dimensions(node map[string]any) (int, int) {
	if dims := asMap(node["dimensions"]); dims != nil {
		return asInt(dims["width"]), asInt(dims["height"])
	}
	return 0, 0
}

func bestImageVersion(item map[string]any) (string, int, int) {
	versions := asMap(item["image_versions2"])
	candidates := asSlice(versions["candidates"])
	best := imageVersionCandidate{
		URL: firstString(
			asString(item["thumbnail_url"]),
			asString(item["display_url"]),
		),
		Width:  asInt(item["original_width"]),
		Height: asInt(item["original_height"]),
	}
	best.Cropped = LooksCroppedMediaURL(best.URL)

	for _, candidate := range candidates {
		node := asMap(candidate)
		if node == nil {
			continue
		}
		url := asString(node["url"])
		if url == "" {
			continue
		}
		next := imageVersionCandidate{
			URL:     url,
			Width:   asInt(node["width"]),
			Height:  asInt(node["height"]),
			Cropped: LooksCroppedMediaURL(url),
		}
		if betterImageCandidate(next, best) {
			best = next
		}
	}

	return best.URL, best.Width, best.Height
}

type imageVersionCandidate struct {
	URL     string
	Width   int
	Height  int
	Cropped bool
}

func betterImageCandidate(candidate, current imageVersionCandidate) bool {
	if candidate.URL == "" {
		return false
	}
	if current.URL == "" {
		return true
	}
	if candidate.Cropped != current.Cropped {
		return !candidate.Cropped
	}
	candidateArea := candidate.Width * candidate.Height
	currentArea := current.Width * current.Height
	if candidateArea != currentArea {
		return candidateArea > currentArea
	}
	return candidate.Width > current.Width
}

func LooksCroppedMediaURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err == nil {
		if stp := parsed.Query().Get("stp"); croppedStpPattern.MatchString(stp) {
			return true
		}
	}
	if _, after, ok := strings.Cut(raw, "stp="); ok {
		stp := after
		if end := strings.IndexByte(stp, '&'); end >= 0 {
			stp = stp[:end]
		}
		if value, err := url.QueryUnescape(stp); err == nil {
			stp = value
		}
		return croppedStpPattern.MatchString(stp)
	}
	return false
}

func bestVideoVersion(item map[string]any) (string, int, int) {
	candidates := asSlice(item["video_versions"])
	bestURL := ""
	bestWidth := 0
	bestHeight := 0
	bestArea := 0

	for _, candidate := range candidates {
		node := asMap(candidate)
		if node == nil {
			continue
		}
		url := asString(node["url"])
		if url == "" {
			continue
		}
		width := asInt(node["width"])
		height := asInt(node["height"])
		area := width * height
		if bestURL == "" || area >= bestArea {
			bestURL = url
			bestWidth = width
			bestHeight = height
			bestArea = area
		}
	}

	return bestURL, bestWidth, bestHeight
}

func applyMetaFallback(post *Post, meta map[string]string) {
	if post.Username == "" {
		post.Username = usernameFromMeta(meta)
	}
	if post.Caption == "" {
		post.Caption = captionFromMeta(meta)
	}

	if post.Username == "" && post.Caption == "" {
		return
	}

	imageURL := firstString(meta["og:image"], meta["twitter:image"])
	videoURL := firstString(meta["og:video"], meta["og:video:secure_url"])
	if len(post.Media) == 0 && videoURL != "" {
		post.Media = append(post.Media, MediaItem{
			Kind:        "video",
			URL:         videoURL,
			PosterURL:   imageURL,
			ContentType: "video/mp4",
		})
		return
	}
	if len(post.Media) == 0 && imageURL != "" {
		post.Media = append(post.Media, MediaItem{
			Kind:        "image",
			URL:         imageURL,
			ContentType: "image/jpeg",
		})
	}
}

func parseMetaTags(body string) map[string]string {
	out := make(map[string]string)
	for _, match := range metaTagPattern.FindAllStringSubmatch(body, -1) {
		attrs := parseAttrs(match[1])
		key := strings.ToLower(firstString(attrs["property"], attrs["name"]))
		content := attrs["content"]
		if key == "" || content == "" {
			continue
		}
		out[key] = html.UnescapeString(content)
	}
	return out
}

func parseAttrs(input string) map[string]string {
	out := make(map[string]string)
	for _, match := range attrPattern.FindAllStringSubmatch(input, -1) {
		value := match[2]
		if value == "" {
			value = match[3]
		}
		out[strings.ToLower(match[1])] = html.UnescapeString(value)
	}
	return out
}

func unmarshalJSONValue(raw string, value any) error {
	if err := json.Unmarshal([]byte(raw), value); err == nil {
		return nil
	}

	decoded, ok := decodeEscapedJSONValue(raw)
	if !ok {
		return json.Unmarshal([]byte(raw), value)
	}
	return json.Unmarshal([]byte(decoded), value)
}

func decodeEscapedJSONValue(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, `\"`) {
		return "", false
	}
	if !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
		return "", false
	}

	var decoded string
	if err := json.Unmarshal([]byte(`"`+raw+`"`), &decoded); err != nil {
		return "", false
	}
	decoded = strings.TrimSpace(decoded)
	if !strings.HasPrefix(decoded, "{") && !strings.HasPrefix(decoded, "[") {
		return "", false
	}
	return decoded, true
}

func extractJSONObjectAfterKey(input, key string) (string, bool) {
	for _, raw := range extractJSONValuesAfterKey(input, key, 1) {
		if strings.HasPrefix(raw, "{") {
			return raw, true
		}
	}
	return "", false
}

func extractJSONValuesAfterKey(input, key string, max int) []string {
	if max <= 0 {
		return nil
	}

	patterns := []string{`"` + key + `"`}
	if key != "items" {
		patterns = append(patterns, key)
	}
	values := make([]string, 0, 1)
	searchStart := 0
	for searchStart < len(input) && len(values) < max {
		relativeIndex := -1
		patternLen := 0
		for _, pattern := range patterns {
			index := strings.Index(input[searchStart:], pattern)
			if index >= 0 && (relativeIndex < 0 || index < relativeIndex) {
				relativeIndex = index
				patternLen = len(pattern)
			}
		}
		if relativeIndex < 0 {
			break
		}

		keyIndex := searchStart + relativeIndex
		afterKey := keyIndex + patternLen
		colonIndex := strings.Index(input[afterKey:], ":")
		if colonIndex < 0 {
			break
		}

		valueStart := afterKey + colonIndex + 1
		for valueStart < len(input) && unicode.IsSpace(rune(input[valueStart])) {
			valueStart++
		}
		if valueStart >= len(input) || (input[valueStart] != '{' && input[valueStart] != '[') {
			searchStart = afterKey
			continue
		}

		end := matchingJSONEnd(input, valueStart)
		if end < 0 {
			searchStart = afterKey
			continue
		}
		values = append(values, input[valueStart:end+1])
		searchStart = end + 1
	}
	return values
}

func matchingObjectEnd(input string, start int) int {
	if start >= len(input) || input[start] != '{' {
		return -1
	}
	return matchingJSONEnd(input, start)
}

func matchingJSONEnd(input string, start int) int {
	stack := make([]byte, 0, 8)
	inString := false
	inEscapedString := false
	escape := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if inEscapedString {
				if escape {
					escape = false
					continue
				}
				if ch == '\\' {
					if i+1 < len(input) && input[i+1] == '"' {
						inString = false
						inEscapedString = false
						i++
						continue
					}
					escape = true
				}
				continue
			}
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '\\':
			if i+1 < len(input) && input[i+1] == '"' {
				inString = true
				inEscapedString = true
				i++
			}
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != ch {
				return -1
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i
			}
		}
	}
	return -1
}

func usernameFromTitle(title string) string {
	return usernameFromText(title)
}

func usernameFromMeta(meta map[string]string) string {
	for _, value := range []string{
		meta["og:title"],
		meta["twitter:title"],
		meta["og:description"],
		meta["twitter:description"],
	} {
		if username := usernameFromText(value); username != "" {
			return username
		}
	}
	return ""
}

func captionFromMeta(meta map[string]string) string {
	value := firstString(meta["og:description"], meta["twitter:description"])
	before, after, ok := strings.Cut(value, ": ")
	if !ok {
		return value
	}
	if _, _, hasDatePrefix := strings.Cut(before, " on "); !hasDatePrefix {
		return value
	}
	after = strings.TrimSpace(after)
	after = strings.TrimPrefix(after, `"`)
	after = strings.TrimSuffix(after, `"`)
	return after
}

func usernameFromText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if match := metaUsernamePattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1]
	}
	if before, _, ok := strings.Cut(value, " on "); ok && ValidUsername(before) {
		return before
	}
	return ""
}

func asMap(value any) map[string]any {
	out, _ := value.(map[string]any)
	return out
}

func asSlice(value any) []any {
	out, _ := value.([]any)
	return out
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return html.UnescapeString(typed)
	default:
		return ""
	}
}

func asBool(value any) bool {
	out, _ := value.(bool)
	return out
}

func asInt(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err == nil {
			return parsed
		}
	}
	return 0
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
