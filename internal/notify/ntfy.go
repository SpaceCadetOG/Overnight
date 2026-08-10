package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	pollHTTP         *http.Client
}

func FromEnvironment() *Client {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("NTFY_URL")), "/")
	topic := strings.Trim(strings.TrimSpace(os.Getenv("NTFY_TOPIC")), "/")
	c := &Client{http: &http.Client{Timeout: 5 * time.Second}, pollHTTP: &http.Client{Timeout: 35 * time.Second}}
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
	return &Client{telegramEndpoint: endpoint, telegramChatID: chatID, http: client, pollHTTP: client}
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

// PollTelegramCommands long-polls the configured bot and accepts commands only
// from TELEGRAM_CHAT_ID. Delivery is observability-only and never gates trading.
func (c *Client) PollTelegramCommands(ctx context.Context, handler func(context.Context, string) (string, string)) {
	if c == nil || c.telegramEndpoint == "" || c.telegramChatID == "" || handler == nil {
		return
	}
	endpoint := strings.TrimSuffix(c.telegramEndpoint, "/sendMessage") + "/getUpdates"
	client := c.pollHTTP
	if client == nil {
		client = &http.Client{Timeout: 35 * time.Second}
	}
	var offset int64
	for ctx.Err() == nil {
		query := url.Values{"timeout": {"20"}, "allowed_updates": {"[\"message\"]"}}
		if offset > 0 {
			query.Set("offset", strconv.FormatInt(offset, 10))
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			if !waitForRetry(ctx) {
				return
			}
			continue
		}
		var payload struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  *struct {
					Text string `json:"text"`
					Chat struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
			} `json:"result"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 || decodeErr != nil || !payload.OK {
			if !waitForRetry(ctx) {
				return
			}
			continue
		}
		for _, update := range payload.Result {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.Message == nil || strconv.FormatInt(update.Message.Chat.ID, 10) != c.telegramChatID {
				continue
			}
			command := telegramCommand(update.Message.Text)
			if command == "" {
				continue
			}
			title, message := handler(ctx, command)
			if title != "" {
				_ = c.sendTelegram(ctx, title, message)
			}
		}
	}
}

func telegramCommand(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	command := strings.ToLower(fields[0])
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return command
}

func waitForRetry(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(5 * time.Second):
		return true
	}
}
