package instagram

import "testing"

func TestParseEmbedHTMLExtractsShortcodeMedia(t *testing.T) {
	ref := Ref{Type: TypeReel, Shortcode: "ABC123xyz"}
	body := `
<html>
  <script>
    window.__data = {"graphql":{"shortcode_media":{
      "owner":{"username":"instafix_user"},
      "edge_media_to_caption":{"edges":[{"node":{"text":"Hello <world>\n\nfrom Instagram"}}]},
      "is_video":true,
      "display_url":"https://scontent.cdninstagram.com/poster.jpg",
      "video_url":"https://scontent.cdninstagram.com/video.mp4",
      "dimensions":{"width":720,"height":1280}
    }}};
  </script>
</html>`

	post, err := ParseEmbedHTML(ref, body)
	if err != nil {
		t.Fatalf("ParseEmbedHTML() error = %v", err)
	}
	if post.Username != "instafix_user" {
		t.Fatalf("Username = %q", post.Username)
	}
	if post.Caption != "Hello <world>\n\nfrom Instagram" {
		t.Fatalf("Caption = %q", post.Caption)
	}
	if len(post.Media) != 1 {
		t.Fatalf("Media length = %d", len(post.Media))
	}
	media := post.Media[0]
	if media.Kind != "video" || media.URL == "" || media.PosterURL == "" {
		t.Fatalf("Media = %#v", media)
	}
}

func TestParseEmbedHTMLUsesMetaFallback(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	body := `
<meta property="og:title" content="@instafix_user on Instagram">
<meta property="og:description" content="Fallback caption">
<meta property="og:image" content="https://scontent.cdninstagram.com/image.jpg">
`

	post, err := ParseEmbedHTML(ref, body)
	if err != nil {
		t.Fatalf("ParseEmbedHTML() error = %v", err)
	}
	if post.Username != "instafix_user" || post.Caption != "Fallback caption" {
		t.Fatalf("post = %#v", post)
	}
	if len(post.Media) != 1 || post.Media[0].Kind != "image" {
		t.Fatalf("media = %#v", post.Media)
	}
}

func TestParseEmbedHTMLExtractsInstagramAPIItemsCarousel(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	body := `
<script>
  window.__data = {"items":[{
    "user":{"username":"loonletwow"},
    "caption":{"text":"The squad coming at you like\n\nPepe: eyes lips eyes"},
    "carousel_media":[{
      "media_type":1,
      "image_versions2":{"candidates":[
        {"url":"https://scontent.cdninstagram.com/one-small.jpg","width":320,"height":320},
        {"url":"https://scontent.cdninstagram.com/one-large.jpg","width":1080,"height":1080}
      ]}
    },{
      "media_type":2,
      "image_versions2":{"candidates":[
        {"url":"https://scontent.cdninstagram.com/two-poster.jpg","width":720,"height":1280}
      ]},
      "video_versions":[
        {"url":"https://scontent.cdninstagram.com/two-video-low.mp4","width":360,"height":640},
        {"url":"https://scontent.cdninstagram.com/two-video-high.mp4","width":720,"height":1280}
      ]
    }]
  }]};
</script>`

	post, err := ParseEmbedHTML(ref, body)
	if err != nil {
		t.Fatalf("ParseEmbedHTML() error = %v", err)
	}
	if post.Username != "loonletwow" {
		t.Fatalf("Username = %q", post.Username)
	}
	if post.Caption != "The squad coming at you like\n\nPepe: eyes lips eyes" {
		t.Fatalf("Caption = %q", post.Caption)
	}
	if len(post.Media) != 2 {
		t.Fatalf("Media length = %d", len(post.Media))
	}
	if post.Media[0].Kind != "image" || post.Media[0].URL != "https://scontent.cdninstagram.com/one-large.jpg" {
		t.Fatalf("First media = %#v", post.Media[0])
	}
	if post.Media[1].Kind != "video" ||
		post.Media[1].URL != "https://scontent.cdninstagram.com/two-video-high.mp4" ||
		post.Media[1].PosterURL != "https://scontent.cdninstagram.com/two-poster.jpg" {
		t.Fatalf("Second media = %#v", post.Media[1])
	}
}
