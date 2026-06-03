package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func ValidateWebhookURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("invalid Discord webhook URL")
	}
	if parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("invalid Discord webhook URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "discord.com" && host != "discordapp.com" {
		return errors.New("invalid Discord webhook host")
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 4 || segments[0] != "api" || segments[1] != "webhooks" || segments[2] == "" || segments[3] == "" {
		return errors.New("invalid Discord webhook path")
	}
	return nil
}

func (c *Client) SendWebhook(ctx context.Context, webhookURL, content string) error {
	if err := ValidateWebhookURL(webhookURL); err != nil {
		return err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("discord message content is required")
	}

	body, err := json.Marshal(map[string]any{
		"content": content,
		"allowed_mentions": map[string]any{
			"parse": []string{},
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "InstaFix/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	message := ""
	if data, readErr := io.ReadAll(io.LimitReader(resp.Body, 512)); readErr == nil {
		message = strings.TrimSpace(string(data))
	}
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("discord webhook returned status %d: %s", resp.StatusCode, message)
}
