package instagram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProfileBlockCooldown(t *testing.T) {
	for _, tt := range []struct{ interval, want time.Duration }{
		{5 * time.Minute, 2 * time.Hour},
		{30 * time.Minute, 2 * time.Hour},
		{2 * time.Hour, 2 * time.Hour},
		{24 * time.Hour, 24 * time.Hour},
	} {
		if got := ProfileBlockCooldown(tt.interval); got != tt.want {
			t.Errorf("cooldown(%v) = %v, want %v", tt.interval, got, tt.want)
		}
	}
}

func TestFetchRecentMediaClassifiesResponses(t *testing.T) {
	for _, tt := range []struct {
		name, body, kind, message string
		status                    int
	}{
		{"unauthorized", "private upstream body", FetchErrorBlocked, "HTTP 401 unauthorized", 401},
		{"forbidden", "private upstream body", FetchErrorBlocked, "HTTP 403 forbidden", 403},
		{"rate limited", "private upstream body", FetchErrorBlocked, "HTTP 429 rate limited", 429},
		{"redirect", "", FetchErrorBlocked, "HTTP 302 redirect", 302},
		{"login", `{"message":"login_required","items":[{"code":"ABC123xyz"}]}`, FetchErrorBlocked, "login required", 200},
		{"login flag", `{"login_required":true}`, FetchErrorBlocked, "login required", 200},
		{"challenge", `{"message":"challenge_required"}`, FetchErrorBlocked, "challenge required", 200},
		{"checkpoint", `{"checkpoint_url":"https://example.invalid/private-token"}`, FetchErrorBlocked, "challenge required", 200},
		{"missing", "", FetchErrorNotFound, "instagram profile not found", 404},
		{"server", "", FetchErrorNetwork, "instagram returned status 503", 503},
		{"html", `<html>private upstream body</html>`, FetchErrorParse, "instagram profile response could not be parsed", 200},
		{"unknown envelope", `{"status":"fail","message":"private upstream body","items":[{"code":"ABC123xyz"}]}`, FetchErrorParse, "instagram profile response reported failure", 200},
		{"empty", `{}`, FetchErrorParse, "instagram profile response could not be parsed", 200},
		{"success", `{"items":[{"code":"ABC123xyz"}]}`, "", "", 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := NewProfileClient(ProfileClientConfig{})
			calls := 0
			client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Location": []string{"https://example.invalid/private-token"}},
					Body:       io.NopCloser(strings.NewReader(tt.body)), Request: req,
				}, nil
			})
			media, err := client.FetchRecentMedia(context.Background(), "example", 12)
			if calls != 1 {
				t.Fatalf("requests = %d, want exactly one (no retries or redirect follow)", calls)
			}
			if tt.kind == "" {
				if err != nil || len(media) != 1 || media[0].Ref.Shortcode != "ABC123xyz" {
					t.Fatalf("success = %v, %v", media, err)
				}
				return
			}
			var fetchErr ProfileFetchError
			if !errors.As(err, &fetchErr) || fetchErr.Kind != tt.kind {
				t.Fatalf("error = %v, want kind %s", err, tt.kind)
			}
			want := tt.message
			if tt.kind == FetchErrorBlocked {
				want = profileBlockedMessage + ": " + want
			}
			if err.Error() != want || len(media) != 0 {
				t.Fatalf("result = %v, %v; want no media and %q", media, err, want)
			}
		})
	}
}

func TestIsProfileBlockedMessage(t *testing.T) {
	for _, message := range []string{profileBlockedMessage, profileBlockedMessage + ": HTTP 429 rate limited", profileBlockedMessage + ": login required"} {
		if !IsProfileBlockedMessage(message) {
			t.Errorf("persisted block not recognized: %q", message)
		}
	}
	if IsProfileBlockedMessage("instagram profile response could not be parsed") {
		t.Fatal("parse failure classified as block")
	}
}

func TestParseProfileMediaJSONExtractsTimelineShortcodes(t *testing.T) {
	body := []byte(`{
  "data": {
    "user": {
      "edge_owner_to_timeline_media": {
        "edges": [{
          "node": {
            "__typename": "GraphImage",
            "shortcode": "ABC123xyz",
            "taken_at_timestamp": 1710000000,
            "edge_media_to_caption": {
              "edges": [{"node": {"text": "Fresh image"}}]
            }
          }
        }, {
          "node": {
            "__typename": "GraphVideo",
            "shortcode": "DEF456xyz",
            "product_type": "clips",
            "taken_at_timestamp": 1710000300,
            "edge_media_to_caption": {
              "edges": [{"node": {"text": "Fresh reel"}}]
            }
          }
        }]
      }
    }
  }
}`)

	media, err := ParseProfileMediaJSON("loonletwow", body, 12)
	if err != nil {
		t.Fatalf("ParseProfileMediaJSON() error = %v", err)
	}
	if len(media) != 2 {
		t.Fatalf("media length = %d", len(media))
	}
	if media[0].Ref.Type != TypePost || media[0].Ref.Shortcode != "ABC123xyz" {
		t.Fatalf("first media ref = %#v", media[0].Ref)
	}
	if media[0].Caption != "Fresh image" {
		t.Fatalf("first caption = %q", media[0].Caption)
	}
	if media[1].Ref.Type != TypeReel || media[1].Ref.Shortcode != "DEF456xyz" {
		t.Fatalf("second media ref = %#v", media[1].Ref)
	}
}

func TestParseProfileMediaJSONUsesItemsFallback(t *testing.T) {
	body := []byte(`{
  "items": [{
    "code": "ABC123xyz",
    "product_type": "feed",
    "taken_at": 1710000000,
    "caption": {"text": "Fallback caption"}
  }]
}`)

	media, err := ParseProfileMediaJSON("loonletwow", body, 12)
	if err != nil {
		t.Fatalf("ParseProfileMediaJSON() error = %v", err)
	}
	if len(media) != 1 {
		t.Fatalf("media length = %d", len(media))
	}
	if media[0].Ref.Type != TypePost || media[0].Ref.Shortcode != "ABC123xyz" {
		t.Fatalf("media ref = %#v", media[0].Ref)
	}
	if media[0].Caption != "Fallback caption" {
		t.Fatalf("caption = %q", media[0].Caption)
	}
}

func TestNormalizeUsername(t *testing.T) {
	username, err := NormalizeUsername("@example.user")
	if err != nil {
		t.Fatalf("NormalizeUsername() error = %v", err)
	}
	if username != "example.user" {
		t.Fatalf("username = %q", username)
	}

	if _, err := NormalizeUsername("bad/user"); err == nil {
		t.Fatal("NormalizeUsername() accepted bad username")
	}
}
