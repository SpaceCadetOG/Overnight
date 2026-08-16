package lighteradapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://mainnet.zklighter.elliot.ai"
	DefaultWSURL   = "wss://mainnet.zklighter.elliot.ai/stream"
)

type Config struct {
	BaseURL           string
	WSURL             string
	HTTPClient        *http.Client
	AuthTokenProvider AuthTokenProvider
}

type Client struct {
	baseURL string
	wsURL   string
	http    *http.Client
	auth    AuthTokenProvider
}

func New(config Config) *Client {
	baseURL := strings.TrimSpace(strings.TrimRight(config.BaseURL, "/"))
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	wsURL := strings.TrimSpace(config.WSURL)
	if wsURL == "" {
		wsURL = DefaultWSURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL: baseURL,
		wsURL:   wsURL,
		http:    httpClient,
		auth:    config.AuthTokenProvider,
	}
}

func (c *Client) authToken() (string, error) {
	if c.auth == nil {
		return "", fmt.Errorf("authenticated access unavailable: no auth token provider configured")
	}
	return c.auth.AuthToken(time.Now().Add(7 * time.Hour))
}

func (c *Client) doGET(ctx context.Context, path string, query url.Values, headers map[string]string, output any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return fmt.Errorf("Lighter %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
