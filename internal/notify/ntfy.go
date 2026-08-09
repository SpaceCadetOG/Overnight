package notify

import (
	"context"
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
	endpoint string
	http     *http.Client
}

func FromEnvironment() *Client {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("NTFY_URL")), "/")
	topic := strings.Trim(strings.TrimSpace(os.Getenv("NTFY_TOPIC")), "/")
	if base == "" || topic == "" {
		return &Client{}
	}
	return &Client{endpoint: base + "/" + url.PathEscape(topic), http: &http.Client{Timeout: 5 * time.Second}}
}

func New(endpoint string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{endpoint: endpoint, http: client}
}

func (c *Client) Enabled() bool { return c != nil && c.endpoint != "" }

func (c *Client) Send(ctx context.Context, title, message, priority, tags string) error {
	if !c.Enabled() {
		return nil
	}
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
