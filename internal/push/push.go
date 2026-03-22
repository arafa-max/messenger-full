package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformIOS     Platform = "ios"
	PlatformWeb     Platform = "web"
)

type Notification struct {
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data,omitempty"`
	ImageURL string         `json:"image_url,omitempty"`
}

type Config struct {
	FCMServerKey string // Firebase Cloud Messaging
	APNsKeyID    string // Apple Push Notification service
	APNsTeamID   string
	APNsBundleID string
	APNsKeyPath  string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send — отправить пуш на одно устройство
func (c *Client) Send(ctx context.Context, token string, platform Platform, n Notification) error {
	switch platform {
	case PlatformAndroid:
		return c.sendFCM(ctx, token, n)
	case PlatformIOS:
		return c.sendFCM(ctx, token, n) // FCM поддерживает iOS через APNs gateway
	case PlatformWeb:
		return c.sendFCM(ctx, token, n)
	default:
		return fmt.Errorf("unknown platform: %s", platform)
	}
}

// SendMulti — отправить пуш на несколько устройств
func (c *Client) SendMulti(ctx context.Context, tokens []TokenPlatform, n Notification) []error {
	errs := make([]error, len(tokens))
	for i, tp := range tokens {
		errs[i] = c.Send(ctx, tp.Token, tp.Platform, n)
	}
	return errs
}

type TokenPlatform struct {
	Token    string
	Platform Platform
}

// ── FCM ───────────────────────────────────────────────────────────────────────

type fcmPayload struct {
	To           string         `json:"to"`
	Notification fcmNotif       `json:"notification"`
	Data         map[string]any `json:"data,omitempty"`
	Priority     string         `json:"priority"`
}

type fcmNotif struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	ImageURL string `json:"image,omitempty"`
}

func (c *Client) sendFCM(ctx context.Context, token string, n Notification) error {
	if c.cfg.FCMServerKey == "" {
		return nil // FCM не настроен — заглушка
	}

	payload := fcmPayload{
		To: token,
		Notification: fcmNotif{
			Title:    n.Title,
			Body:     n.Body,
			ImageURL: n.ImageURL,
		},
		Data:     n.Data,
		Priority: "high",
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://fcm.googleapis.com/fcm/send", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+c.cfg.FCMServerKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM error: status %d", resp.StatusCode)
	}
	return nil
}

// IsConfigured — проверить настроен ли push сервис
func (c *Client) IsConfigured() bool {
	return c.cfg.FCMServerKey != ""
}