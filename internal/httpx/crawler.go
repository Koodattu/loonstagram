package httpx

import (
	"net/http"
	"strings"
)

var crawlerFragments = []string{
	"discordbot",
	"discord",
	"facebookexternalhit",
	"twitterbot",
	"slackbot",
	"telegrambot",
	"whatsapp",
	"linkedinbot",
	"embedly",
	"preview",
	"bot",
	"crawl",
	"spider",
}

func IsCrawler(r *http.Request) bool {
	query := r.URL.Query()
	if query.Get("redirect") == "1" {
		return false
	}
	if query.Get("preview") == "1" {
		return true
	}

	ua := strings.ToLower(r.UserAgent())
	if ua == "" {
		return false
	}
	for _, fragment := range crawlerFragments {
		if strings.Contains(ua, fragment) {
			return true
		}
	}
	return false
}

func UserAgentCategory(r *http.Request) string {
	if IsCrawler(r) {
		return "crawler"
	}
	return "human"
}
