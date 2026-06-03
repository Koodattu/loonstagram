package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestIsCrawler(t *testing.T) {
	req := httptest.NewRequest("GET", "/p/ABC123xyz", nil)
	req.Header.Set("User-Agent", "Discordbot/2.0")
	if !IsCrawler(req) {
		t.Fatal("Discordbot should be treated as crawler")
	}

	req = httptest.NewRequest("GET", "/p/ABC123xyz", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/124.0")
	if IsCrawler(req) {
		t.Fatal("regular Chrome should be treated as human")
	}

	req = httptest.NewRequest("GET", "/p/ABC123xyz?preview=1", nil)
	if !IsCrawler(req) {
		t.Fatal("preview=1 should force crawler")
	}

	req = httptest.NewRequest("GET", "/p/ABC123xyz?redirect=1", nil)
	req.Header.Set("User-Agent", "Discordbot/2.0")
	if IsCrawler(req) {
		t.Fatal("redirect=1 should force human")
	}
}
