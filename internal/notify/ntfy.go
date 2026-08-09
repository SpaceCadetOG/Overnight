package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client is a best-effort ntfy publisher. Notifications are observability
// only: callers must never make trading or recording depend on delivery.
type Client struct {
	endpoint         string
	telegramEndpoint string
	telegramChatID   string
	http             *http.Client
}

func FromEnvironment() *Client {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("NTFY_URL")), "/")
	topic := strings.Trim(strings.TrimSpace(os.Getenv("NTFY_TOPIC")), "/")
	c := &Client{http: &http.Client{Timeout: 5 * time.Second}}
	if base != "" && topic != "" {
		c.endpoint = base + "/" + url.PathEscape(topic)
	}
	if token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); token != "" {
		if chatID := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")); chatID != "" {
			c.telegramEndpoint = "https://api.telegram.org/bot" + token + "/sendMessage"
			c.telegramChatID = chatID
		}
	}
	return c
}

func New(endpoint string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{endpoint: endpoint, http: client}
}

func NewTelegram(endpoint, chatID string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{telegramEndpoint: endpoint, telegramChatID: chatID, http: client}
}

func (c *Client) Enabled() bool {
	return c != nil && (c.endpoint != "" || (c.telegramEndpoint != "" && c.telegramChatID != ""))
}

func (c *Client) Send(ctx context.Context, title, message, priority, tags string) error {
	if !c.Enabled() {
		return nil
	}
	var firstErr error
	if c.endpoint != "" {
		if err := c.sendNtfy(ctx, title, message, priority, tags); err != nil {
			firstErr = err
		}
	}
	if c.telegramEndpoint != "" && c.telegramChatID != "" {
		if err := c.sendTelegram(ctx, title, message); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Client) sendNtfy(ctx context.Context, title, message, priority, tags string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if priority != "" {
		req.Header.Set("Priority", priority)
	}
	if tags != "" {
		req.Header.Set("Tags", tags)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %s", resp.Status)
	}
	return nil
}

func (c *Client) sendTelegram(ctx context.Context, title, message string) error {
	body, err := json.Marshal(map[string]string{
		"chat_id": c.telegramChatID,
		"text":    title + "\n" + message,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.telegramEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned %s", resp.Status)
	}
	return nil
}

func (c *Client) BestEffort(title, message, priority, tags string) {
	if !c.Enabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		_ = c.Send(ctx, title, message, priority, tags)
	}()
}
