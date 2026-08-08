package lighterexec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	lighterclient "github.com/elliottech/lighter-go/client"
	lighterhttp "github.com/elliottech/lighter-go/client/http"
	lightertypes "github.com/elliottech/lighter-go/types"
	"github.com/gorilla/websocket"
)

const (
	defaultBaseURL = "https://mainnet.zklighter.elliot.ai"
	defaultWSURL   = "wss://mainnet.zklighter.elliot.ai/stream"
)

type Config struct {
	BaseURL, WSURL, PrivateKey string
	AccountIndex               int64
	APIKeyIndex                uint8
	ChainID                    uint32
}

type Market struct {
	Symbol         string `json:"symbol"`
	MarketID       int16  `json:"market_id"`
	Status         string `json:"status"`
	MinBaseAmount  any    `json:"min_base_amount"`
	MinQuoteAmount any    `json:"min_quote_amount"`
	PriceDecimals  int    `json:"supported_price_decimals"`
	SizeDecimals   int    `json:"supported_size_decimals"`
	MarkPrice      any    `json:"mark_price"`
}

type Snapshot struct {
	Account   map[string]any
	Orders    []map[string]any
	Fills     []map[string]any
	Positions []map[string]any
}

type Client struct {
	cfg    Config
	http   *http.Client
	signer *lighterclient.TxClient
}

func CheckPublic(ctx context.Context, baseURL string) ([]Market, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	var response struct {
		Markets []Market `json:"order_book_details"`
	}
	client := &Client{cfg: Config{BaseURL: strings.TrimRight(baseURL, "/")}, http: &http.Client{Timeout: 15 * time.Second}}
	if err := client.get(ctx, "/api/v1/orderBookDetails", nil, "", &response); err != nil {
		return nil, err
	}
	return response.Markets, nil
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if strings.TrimSpace(cfg.WSURL) == "" {
		cfg.WSURL = defaultWSURL
	}
	if cfg.ChainID == 0 {
		cfg.ChainID = 304
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.AccountIndex <= 0 {
		return nil, fmt.Errorf("LIGHTER_ACCOUNT_INDEX must be positive")
	}
	if strings.TrimSpace(cfg.PrivateKey) == "" {
		return nil, fmt.Errorf("LIGHTER_API_PRIVATE_KEY is required")
	}
	signer, err := lighterclient.NewTxClient(lighterhttp.NewClient(cfg.BaseURL), strings.TrimSpace(cfg.PrivateKey), cfg.AccountIndex, cfg.APIKeyIndex, cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("initialize Lighter authentication: %w", err)
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}, signer: signer}, nil
}

func (c *Client) CheckCredentials() error { return c.signer.Check() }

// ValidateCreateOrder signs and validates an exact order payload locally with
// an explicit nonce. It never sends the transaction to Lighter.
func (c *Client) ValidateCreateOrder(req *lightertypes.CreateOrderTxReq, nonce int64) error {
	ops := &lightertypes.TransactOpts{Nonce: &nonce}
	signed, err := c.signer.GetCreateOrderTransaction(req, ops)
	if err != nil {
		return err
	}
	return signed.Validate()
}

func (c *Client) authToken() (string, error) {
	return c.signer.GetAuthToken(time.Now().Add(7 * time.Hour))
}

func (c *Client) ReadSnapshot(ctx context.Context, markets []Market) (Snapshot, error) {
	token, err := c.authToken()
	if err != nil {
		return Snapshot{}, err
	}
	accountIndex := strconv.FormatInt(c.cfg.AccountIndex, 10)
	var accounts struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := c.get(ctx, "/api/v1/account", url.Values{"by": {"index"}, "value": {accountIndex}}, "", &accounts); err != nil {
		return Snapshot{}, err
	}
	if len(accounts.Accounts) == 0 {
		return Snapshot{}, fmt.Errorf("Lighter account %d not found", c.cfg.AccountIndex)
	}
	snapshot := Snapshot{Account: accounts.Accounts[0]}
	if positions, ok := accounts.Accounts[0]["positions"].([]any); ok {
		for _, value := range positions {
			if position, ok := value.(map[string]any); ok {
				snapshot.Positions = append(snapshot.Positions, position)
			}
		}
	}
	var trades struct {
		Trades []map[string]any `json:"trades"`
	}
	tradeQuery := url.Values{"market_id": {"255"}, "market_type": {"all"}, "account_index": {accountIndex}, "sort_by": {"timestamp"}, "sort_dir": {"desc"}, "limit": {"100"}}
	if err := c.get(ctx, "/api/v1/trades", tradeQuery, token, &trades); err != nil {
		return Snapshot{}, err
	}
	snapshot.Fills = trades.Trades
	for _, market := range markets {
		if strings.EqualFold(market.Status, "inactive") {
			continue
		}
		var orders struct {
			Orders []map[string]any `json:"orders"`
		}
		query := url.Values{"account_index": {accountIndex}, "market_id": {strconv.Itoa(int(market.MarketID))}, "auth": {token}}
		if err := c.get(ctx, "/api/v1/accountActiveOrders", query, "", &orders); err != nil {
			return Snapshot{}, err
		}
		snapshot.Orders = append(snapshot.Orders, orders.Orders...)
	}
	return snapshot, nil
}

func (c *Client) CheckPrivateWebSocket(ctx context.Context) error {
	token, err := c.authToken()
	if err != nil {
		return err
	}
	headers := http.Header{"Origin": []string{"https://lighter.xyz"}, "User-Agent": []string{"overnight-strategy-check/1.0"}}
	var conn *websocket.Conn
	for attempt := 1; attempt <= 3; attempt++ {
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.cfg.WSURL, headers)
		if err == nil {
			break
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	if err != nil {
		return fmt.Errorf("connect private WebSocket after 3 attempts: %w", err)
	}
	defer conn.Close()
	channel := "account_all_positions/" + strconv.FormatInt(c.cfg.AccountIndex, 10)
	if err := conn.WriteJSON(map[string]any{"type": "subscribe", "channel": channel, "auth": token}); err != nil {
		return fmt.Errorf("subscribe private WebSocket: %w", err)
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetReadDeadline(deadline)
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read private WebSocket: %w", err)
	}
	var event map[string]any
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("decode private WebSocket response: %w", err)
	}
	if errorValue := strings.TrimSpace(fmt.Sprint(event["error"])); errorValue != "" && errorValue != "<nil>" {
		return fmt.Errorf("private WebSocket rejected subscription: %s", errorValue)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, auth string, output any) error {
	endpoint := c.cfg.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("Lighter %s returned HTTP %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(output)
}
