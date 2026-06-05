package httpx

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/discord"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/secret"
)

type automationStatusResponse struct {
	OK                     bool   `json:"ok"`
	AdminConfigured        bool   `json:"adminConfigured"`
	DiscordOAuthConfigured bool   `json:"discordOAuthConfigured"`
	InstagramUsername      string `json:"instagramUsername"`
	Enabled                bool   `json:"enabled"`
	DiscordConnected       bool   `json:"discordConnected"`
	DiscordLabel           string `json:"discordLabel,omitempty"`
	LastCheckedAt          string `json:"lastCheckedAt,omitempty"`
	LastPostedAt           string `json:"lastPostedAt,omitempty"`
	LastStatus             string `json:"lastStatus,omitempty"`
	LastError              string `json:"lastError,omitempty"`
}

type automationConfigRequest struct {
	InstagramUsername string `json:"instagramUsername"`
	Enabled           bool   `json:"enabled"`
}

type discordWebhookRequest struct {
	WebhookURL string `json:"webhookUrl"`
}

type automationResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (h *Handlers) automationStatus(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not load automation settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.automationStatusPayload(settings))
}

func (h *Handlers) saveAutomationConfig(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()

	var req automationConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Invalid automation settings"})
		return
	}

	username := strings.TrimSpace(req.InstagramUsername)
	if username != "" {
		normalized, err := instagram.NormalizeUsername(username)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Unsupported Instagram username"})
			return
		}
		username = normalized
	}
	if req.Enabled && username == "" {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Instagram username is required"})
		return
	}

	if err := h.store.SaveAutomationConfig(r.Context(), username, req.Enabled, time.Now()); err != nil {
		h.logger.Error("save automation config", "error", err)
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not save automation settings"})
		return
	}
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not load automation settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.automationStatusPayload(settings))
}

func (h *Handlers) saveDiscordWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()

	var req discordWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Invalid Discord webhook"})
		return
	}
	webhookURL := strings.TrimSpace(req.WebhookURL)
	if err := discord.ValidateWebhookURL(webhookURL); err != nil {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: err.Error()})
		return
	}
	sealedURL, err := secret.SealString(h.adminToken, webhookURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not protect Discord webhook"})
		return
	}

	if err := h.store.SetDiscordWebhook(r.Context(), cache.DiscordWebhook{
		URL:  sealedURL,
		Name: "Manual webhook",
	}, time.Now()); err != nil {
		h.logger.Error("save discord webhook", "error", err)
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not save Discord webhook"})
		return
	}
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not load automation settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.automationStatusPayload(settings))
}

func (h *Handlers) disconnectDiscordWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	if err := h.store.ClearDiscordWebhook(r.Context(), time.Now()); err != nil {
		h.logger.Error("disconnect discord webhook", "error", err)
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not disconnect Discord"})
		return
	}
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not load automation settings"})
		return
	}
	writeJSON(w, http.StatusOK, h.automationStatusPayload(settings))
}

func (h *Handlers) testDiscordWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not load automation settings"})
		return
	}
	if settings.DiscordWebhookURL == "" {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Discord is not connected"})
		return
	}
	webhookURL, err := secret.OpenString(h.adminToken, settings.DiscordWebhookURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not read Discord webhook"})
		return
	}
	client := discord.NewClient(8 * time.Second)
	if err := client.SendWebhook(r.Context(), webhookURL, "Loonstagram Discord delivery test: "+h.publicBaseURL); err != nil {
		writeJSON(w, http.StatusBadGateway, automationResponse{OK: false, Error: "Discord test failed"})
		return
	}
	writeJSON(w, http.StatusOK, automationResponse{OK: true})
}

func (h *Handlers) startDiscordOAuth(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(w, r) {
		return
	}
	if !h.discordOAuthConfigured() {
		writeJSON(w, http.StatusBadRequest, automationResponse{OK: false, Error: "Discord OAuth is not configured"})
		return
	}

	state, err := randomState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not start Discord connection"})
		return
	}
	now := time.Now()
	if err := h.store.CreateDiscordOAuthState(r.Context(), state, now, now.Add(10*time.Minute)); err != nil {
		h.logger.Error("create discord oauth state", "error", err)
		writeJSON(w, http.StatusInternalServerError, automationResponse{OK: false, Error: "Could not start Discord connection"})
		return
	}

	values := url.Values{}
	values.Set("client_id", h.discordClientID)
	values.Set("redirect_uri", h.discordCallbackURL())
	values.Set("response_type", "code")
	values.Set("scope", "webhook.incoming")
	values.Set("state", state)
	http.Redirect(w, r, "https://discord.com/oauth2/authorize?"+values.Encode(), http.StatusFound)
}

func (h *Handlers) discordOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !h.discordOAuthConfigured() {
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}
	ok, err := h.store.ConsumeDiscordOAuthState(r.Context(), state, time.Now())
	if err != nil || !ok {
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}

	webhook, err := h.exchangeDiscordCode(r, code)
	if err != nil {
		h.logger.Warn("discord oauth exchange failed", "error", sanitizeLogError(err))
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}
	webhook.URL, err = secret.SealString(h.adminToken, webhook.URL)
	if err != nil {
		h.logger.Error("protect discord oauth webhook", "error", err)
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}
	if err := h.store.SetDiscordWebhook(r.Context(), webhook, time.Now()); err != nil {
		h.logger.Error("save discord oauth webhook", "error", err)
		http.Redirect(w, r, "/admin?discord=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin?discord=connected", http.StatusFound)
}

func (h *Handlers) exchangeDiscordCode(r *http.Request, code string) (cache.DiscordWebhook, error) {
	values := url.Values{}
	values.Set("client_id", h.discordClientID)
	values.Set("client_secret", h.discordClientSecret)
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", h.discordCallbackURL())

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://discord.com/api/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return cache.DiscordWebhook{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Loonstagram/1.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return cache.DiscordWebhook{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return cache.DiscordWebhook{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return cache.DiscordWebhook{}, fmt.Errorf("discord token exchange returned status %d", resp.StatusCode)
	}

	var payload struct {
		Webhook struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			ChannelID string `json:"channel_id"`
			GuildID   string `json:"guild_id"`
			URL       string `json:"url"`
		} `json:"webhook"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		return cache.DiscordWebhook{}, err
	}
	if payload.Webhook.URL == "" {
		return cache.DiscordWebhook{}, errors.New("discord did not return a webhook URL")
	}
	if err := discord.ValidateWebhookURL(payload.Webhook.URL); err != nil {
		return cache.DiscordWebhook{}, err
	}
	return cache.DiscordWebhook{
		URL:       payload.Webhook.URL,
		ID:        payload.Webhook.ID,
		Name:      payload.Webhook.Name,
		ChannelID: payload.Webhook.ChannelID,
		GuildID:   payload.Webhook.GuildID,
	}, nil
}

func (h *Handlers) automationStatusPayload(settings cache.AutomationSettings) automationStatusResponse {
	label := firstString(settings.DiscordChannelName, settings.DiscordWebhookName, settings.DiscordChannelID)
	return automationStatusResponse{
		OK:                     true,
		AdminConfigured:        h.adminToken != "",
		DiscordOAuthConfigured: h.discordOAuthConfigured(),
		InstagramUsername:      settings.InstagramUsername,
		Enabled:                settings.Enabled,
		DiscordConnected:       settings.DiscordWebhookURL != "",
		DiscordLabel:           label,
		LastCheckedAt:          formatTime(settings.LastCheckedAt),
		LastPostedAt:           formatTime(settings.LastPostedAt),
		LastStatus:             settings.LastStatus,
		LastError:              settings.LastError,
	}
}

func (h *Handlers) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.adminToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, automationResponse{OK: false, Error: "ADMIN_TOKEN is not configured"})
		return false
	}

	token := r.Header.Get("X-Admin-Token")
	if token == "" {
		const bearerPrefix = "Bearer "
		if value := r.Header.Get("Authorization"); strings.HasPrefix(value, bearerPrefix) {
			token = strings.TrimSpace(strings.TrimPrefix(value, bearerPrefix))
		}
	}
	if token == "" {
		token = r.URL.Query().Get("admin_token")
	}

	if len(token) != len(h.adminToken) || subtle.ConstantTimeCompare([]byte(token), []byte(h.adminToken)) != 1 {
		writeJSON(w, http.StatusForbidden, automationResponse{OK: false, Error: "Forbidden"})
		return false
	}
	return true
}

func (h *Handlers) discordOAuthConfigured() bool {
	return h.discordClientID != "" && h.discordClientSecret != ""
}

func (h *Handlers) discordCallbackURL() string {
	if h.discordRedirectURL != "" {
		return h.discordRedirectURL
	}
	return h.publicURL("/oauth/discord/callback")
}

func randomState() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
