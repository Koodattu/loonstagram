package instagram

import "testing"

func TestNormalizeURLAcceptsSupportedPaths(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  string
		wantCode  string
	}{
		{
			name:     "post",
			input:    "https://www.instagram.com/p/ABC123xyz/",
			wantType: TypePost,
			wantCode: "ABC123xyz",
		},
		{
			name:     "reel",
			input:    "https://instagram.com/reel/ABC123xyz/?igsh=test",
			wantType: TypeReel,
			wantCode: "ABC123xyz",
		},
		{
			name:     "reels alias",
			input:    "https://www.instagram.com/reels/ABC123xyz/",
			wantType: TypeReel,
			wantCode: "ABC123xyz",
		},
		{
			name:     "tv",
			input:    "https://www.instagram.com/tv/ABC123xyz/",
			wantType: TypeTV,
			wantCode: "ABC123xyz",
		},
		{
			name:     "username post",
			input:    "https://www.instagram.com/example.user/p/ABC123xyz/",
			wantType: TypePost,
			wantCode: "ABC123xyz",
		},
		{
			name:     "username reel",
			input:    "https://www.instagram.com/example.user/reel/ABC123xyz/",
			wantType: TypeReel,
			wantCode: "ABC123xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := NormalizeURL(tt.input)
			if err != nil {
				t.Fatalf("NormalizeURL() error = %v", err)
			}
			if ref.Type != tt.wantType || ref.Shortcode != tt.wantCode {
				t.Fatalf("NormalizeURL() = %#v, want type %q shortcode %q", ref, tt.wantType, tt.wantCode)
			}
		})
	}
}

func TestNormalizeURLRejectsUnsupportedInput(t *testing.T) {
	tests := []string{
		"https://example.com/p/ABC123xyz/",
		"http://www.instagram.com/p/ABC123xyz/",
		"https://www.instagram.com/stories/example/123/",
		"https://www.instagram.com/p/abc/",
		"https://www.instagram.com/p/ABC123xyz%2Fextra/",
	}

	for _, input := range tests {
		if _, err := NormalizeURL(input); err == nil {
			t.Fatalf("NormalizeURL(%q) succeeded, want error", input)
		}
	}
}
