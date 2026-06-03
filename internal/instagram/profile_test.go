package instagram

import "testing"

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
