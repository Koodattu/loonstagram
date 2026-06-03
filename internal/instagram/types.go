package instagram

import (
	"errors"
	"fmt"
	"time"
)

const (
	TypePost = "p"
	TypeReel = "reel"
	TypeTV   = "tv"
)

var ErrUnsupportedURL = errors.New("unsupported Instagram URL")

type Ref struct {
	Type      string
	Shortcode string
}

type MediaItem struct {
	Kind        string `json:"kind"`
	URL         string `json:"url,omitempty"`
	PosterURL   string `json:"posterUrl,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

type Post struct {
	Ref         Ref
	OriginalURL string
	Username    string
	Caption     string
	Media       []MediaItem
	Status      string
	Error       string
	FetchedAt   time.Time
	ExpiresAt   time.Time
}

func NewRef(mediaType, shortcode string) (Ref, error) {
	mediaType = CanonicalType(mediaType)
	if !IsSupportedType(mediaType) || !ValidShortcode(shortcode) {
		return Ref{}, ErrUnsupportedURL
	}
	return Ref{Type: mediaType, Shortcode: shortcode}, nil
}

func IsSupportedType(mediaType string) bool {
	switch mediaType {
	case TypePost, TypeReel, TypeTV:
		return true
	default:
		return false
	}
}

func CanonicalType(mediaType string) string {
	if mediaType == "reels" {
		return TypeReel
	}
	return mediaType
}

func (r Ref) OriginalURL() string {
	mediaType := r.Type
	if mediaType == "" {
		mediaType = TypePost
	}
	return fmt.Sprintf("https://www.instagram.com/%s/%s/", mediaType, r.Shortcode)
}

func (r Ref) EmbedURL() string {
	mediaType := r.Type
	if mediaType == "" {
		mediaType = TypePost
	}
	return fmt.Sprintf("https://www.instagram.com/%s/%s/embed/captioned/", mediaType, r.Shortcode)
}

func (r Ref) CanonicalPath() string {
	return fmt.Sprintf("/%s/%s", r.Type, r.Shortcode)
}

func FallbackPost(ref Ref, status, reason string) *Post {
	return &Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Status:      status,
		Error:       reason,
	}
}
