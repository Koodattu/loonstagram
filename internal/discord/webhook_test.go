package discord

import "testing"

func TestValidateWebhookURL(t *testing.T) {
	valid := []string{
		"https://discord.com/api/webhooks/123/token",
		"https://discordapp.com/api/webhooks/123/token",
	}
	for _, input := range valid {
		if err := ValidateWebhookURL(input); err != nil {
			t.Fatalf("ValidateWebhookURL(%q) error = %v", input, err)
		}
	}

	invalid := []string{
		"http://discord.com/api/webhooks/123/token",
		"https://example.com/api/webhooks/123/token",
		"https://discord.com/api/other/123/token",
		"https://discord.com/api/webhooks/123/token?wait=true",
		"https://discord.com/api/webhooks/123/token/slack",
	}
	for _, input := range invalid {
		if err := ValidateWebhookURL(input); err == nil {
			t.Fatalf("ValidateWebhookURL(%q) succeeded", input)
		}
	}
}
