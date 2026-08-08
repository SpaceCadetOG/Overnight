package lighter

import (
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
	auth    string
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if baseURL == "" {
		baseURL = "https://mainnet.zklighter.elliot.ai"
	}

	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

func (c *Client) SetAuth(token string) {
	c.auth = token
}
