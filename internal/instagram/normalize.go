package instagram

import (
	"net/url"
	"regexp"
	"strings"
)

var shortcodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{5,128}$`)

func ValidShortcode(shortcode string) bool {
	return shortcodePattern.MatchString(shortcode)
}

func NormalizeURL(raw string) (Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Ref{}, ErrUnsupportedURL
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" {
		return Ref{}, ErrUnsupportedURL
	}

	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "instagram.com", "www.instagram.com", "m.instagram.com":
	default:
		return Ref{}, ErrUnsupportedURL
	}

	segments := splitPath(parsed.EscapedPath())
	if len(segments) < 2 {
		return Ref{}, ErrUnsupportedURL
	}

	if ref, ok := refFromSegments(segments); ok {
		return ref, nil
	}

	if len(segments) >= 3 {
		if ref, ok := refFromSegments(segments[1:]); ok {
			return ref, nil
		}
	}

	return Ref{}, ErrUnsupportedURL
}

func splitPath(path string) []string {
	rawSegments := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		if segment == "" {
			continue
		}
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return nil
		}
		segments = append(segments, decoded)
	}
	return segments
}

func refFromSegments(segments []string) (Ref, bool) {
	if len(segments) < 2 {
		return Ref{}, false
	}
	mediaType := CanonicalType(segments[0])
	if !IsSupportedType(mediaType) {
		return Ref{}, false
	}
	ref, err := NewRef(mediaType, segments[1])
	return ref, err == nil
}

func ConvertURL(publicBaseURL, raw string) (string, Ref, error) {
	ref, err := NormalizeURL(raw)
	if err != nil {
		return "", Ref{}, err
	}
	return strings.TrimRight(publicBaseURL, "/") + ref.CanonicalPath(), ref, nil
}
