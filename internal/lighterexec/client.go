package lighterexec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	lighterclient "github.com/elliottech/lighter-go/client"
	lighterhttp "github.com/elliottech/lighter-go/client/http"
	lightertypes "github.com/elliottech/lighter-go/types"
	lighteradapter "github.com/ogtrading/lighter-adapter"
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
	Account          map[string]any            `json:"account"`
	Orders           []map[string]any          `json:"active_orders"`
	HistoricalOrders []map[string]any          `json:"historical_orders"`
	Fills            []map[string]any          `json:"fills"`
	FundingPayments  []map[string]any          `json:"funding_payments"`
	Positions        []map[string]any          `json:"positions"`
	Endpoints        map[string]EndpointStatus `json:"endpoints"`
}

type EndpointStatus struct {
	State       string    `json:"state"`
	RetrievedAt time.Time `json:"retrieved_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type Client struct {
	cfg                                          Config
	http                                         *http.Client
	signer                                       *lighterclient.TxClient
	authTokenFn                                  func() (string, error)
	readonly                                     *lighteradapter.Client
	mu                                           sync.Mutex
	cachedFills, cachedHistorical, cachedFunding []map[string]any
	fillsAt, historicalAt, fundingAt             time.Time
}

func CheckPublic(ctx context.Context, baseURL string) ([]Market, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	result, err := lighteradapter.New(lighteradapter.Config{
		BaseURL: baseURL,
	}).Markets(ctx)
	if err != nil {
		return nil, err
	}
	markets := make([]Market, 0, len(result.Items))
	for _, item := range result.Items {
		markets = append(markets, fromAdapterMarket(item))
	}
	return markets, nil
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
	tokenProvider := lighteradapter.AuthTokenFunc(func(expiresAt time.Time) (string, error) {
		return signer.GetAuthToken(expiresAt)
	})
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 15 * time.Second},
		signer: signer,
		readonly: lighteradapter.New(lighteradapter.Config{
			BaseURL:           cfg.BaseURL,
			WSURL:             cfg.WSURL,
			HTTPClient:        &http.Client{Timeout: 15 * time.Second},
			AuthTokenProvider: tokenProvider,
		}),
	}, nil
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
	if c.authTokenFn != nil {
		return c.authTokenFn()
	}
	return c.signer.GetAuthToken(time.Now().Add(7 * time.Hour))
}

func (c *Client) ReadSnapshot(ctx context.Context, markets []Market) (Snapshot, error) {
	snapshot, err := c.ReadReconciliationSnapshot(ctx, markets)
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// ReadReconciliationSnapshot independently scores the four authoritative
// endpoint groups. A failed endpoint is never represented as a fresh empty
// collection. Expensive fill/history endpoints are refreshed at most once per
// minute and remain usable for two minutes.
func (c *Client) ReadReconciliationSnapshot(ctx context.Context, markets []Market) (Snapshot, error) {
	now := time.Now().UTC()
	snapshot := Snapshot{Endpoints: map[string]EndpointStatus{}}
	failures := []string{}
	accountIndex := c.cfg.AccountIndex
	accounts, err := c.readonly.AccountByIndex(ctx, accountIndex)
	if err != nil {
		snapshot.Endpoints["positions"] = EndpointStatus{State: "failed", Error: err.Error()}
		failures = append(failures, "positions")
	} else if len(accounts.Items) == 0 {
		snapshot.Endpoints["positions"] = EndpointStatus{State: "failed", Error: "account not found"}
		failures = append(failures, "positions")
	} else {
		snapshot.Account = toMap(accounts.Items[0])
		snapshot.Positions = toMapSlice(accounts.Items[0].Positions)
		snapshot.Endpoints["positions"] = EndpointStatus{State: "fresh", RetrievedAt: now}
	}

	activeOK := true
	for _, market := range markets {
		if strings.EqualFold(market.Status, "inactive") {
			continue
		}
		orders, err := c.readonly.ActiveOrders(ctx, accountIndex, int(market.MarketID))
		if err != nil {
			activeOK = false
			failures = append(failures, "active_orders")
			snapshot.Endpoints["active_orders"] = EndpointStatus{State: "failed", Error: err.Error()}
			break
		}
		snapshot.Orders = append(snapshot.Orders, toMapSlice(orders.Items)...)
	}
	if activeOK {
		snapshot.Endpoints["active_orders"] = EndpointStatus{State: "fresh", RetrievedAt: now}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fillsAt.IsZero() || now.Sub(c.fillsAt) >= time.Minute {
		trades, err := c.readonly.Fills(ctx, accountIndex, 100)
		if err != nil {
			if c.fillsAt.IsZero() || now.Sub(c.fillsAt) > 2*time.Minute {
				snapshot.Endpoints["fills"] = EndpointStatus{State: "failed", Error: err.Error()}
				failures = append(failures, "fills")
			} else {
				snapshot.Endpoints["fills"] = EndpointStatus{State: "stale", RetrievedAt: c.fillsAt, Error: err.Error()}
				failures = append(failures, "fills")
			}
		} else {
			c.cachedFills, c.fillsAt = toMapSlice(trades.Items), now
		}
	}
	snapshot.Fills = append(snapshot.Fills, c.cachedFills...)
	if _, ok := snapshot.Endpoints["fills"]; !ok {
		snapshot.Endpoints["fills"] = EndpointStatus{State: "fresh", RetrievedAt: c.fillsAt}
	}

	if c.historicalAt.IsZero() || now.Sub(c.historicalAt) >= time.Minute {
		historical := []map[string]any{}
		historyOK := true
		var historyErr error
		for _, market := range markets {
			if strings.EqualFold(market.Status, "inactive") {
				continue
			}
			orders, err := c.readonly.HistoricalOrders(ctx, accountIndex, int(market.MarketID), 100)
			if err != nil {
				historyOK = false
				historyErr = err
				break
			}
			historical = append(historical, toMapSlice(orders.Items)...)
		}
		if historyOK {
			c.cachedHistorical, c.historicalAt = historical, now
		} else if c.historicalAt.IsZero() || now.Sub(c.historicalAt) > 2*time.Minute {
			snapshot.Endpoints["historical_orders"] = EndpointStatus{State: "failed", Error: historyErr.Error()}
			failures = append(failures, "historical_orders")
		} else {
			snapshot.Endpoints["historical_orders"] = EndpointStatus{State: "stale", RetrievedAt: c.historicalAt, Error: historyErr.Error()}
			failures = append(failures, "historical_orders")
		}
	}
	snapshot.HistoricalOrders = append(snapshot.HistoricalOrders, c.cachedHistorical...)
	if _, ok := snapshot.Endpoints["historical_orders"]; !ok {
		snapshot.Endpoints["historical_orders"] = EndpointStatus{State: "fresh", RetrievedAt: c.historicalAt}
	}
	if c.fundingAt.IsZero() || now.Sub(c.fundingAt) >= time.Minute {
		funding, err := c.readonly.Funding(ctx, accountIndex, 100, "all")
		if err != nil {
			if c.fundingAt.IsZero() || now.Sub(c.fundingAt) > 2*time.Minute {
				snapshot.Endpoints["funding"] = EndpointStatus{State: "failed", Error: err.Error()}
				failures = append(failures, "funding")
			} else {
				snapshot.Endpoints["funding"] = EndpointStatus{State: "stale", RetrievedAt: c.fundingAt, Error: err.Error()}
				failures = append(failures, "funding")
			}
		} else {
			c.cachedFunding = toMapSlice(funding.Items)
			c.fundingAt = now
		}
	}
	snapshot.FundingPayments = append(snapshot.FundingPayments, c.cachedFunding...)
	if _, ok := snapshot.Endpoints["funding"]; !ok {
		snapshot.Endpoints["funding"] = EndpointStatus{State: "fresh", RetrievedAt: c.fundingAt}
	}
	if len(failures) > 0 {
		return snapshot, fmt.Errorf("authoritative reconciliation incomplete: %s", strings.Join(uniqueStrings(failures), ","))
	}
	return snapshot, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (c *Client) CheckPrivateWebSocket(ctx context.Context) error {
	_, err := c.readonly.CheckPrivateWebSocket(ctx, c.cfg.WSURL, c.cfg.AccountIndex)
	return err
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

func fromAdapterMarket(m lighteradapter.Market) Market {
	return Market{
		Symbol:         m.Symbol,
		MarketID:       m.MarketID,
		Status:         m.Status,
		MinBaseAmount:  m.MinBaseAmount.String(),
		MinQuoteAmount: m.MinQuoteAmount.String(),
		PriceDecimals:  m.PriceDecimals,
		SizeDecimals:   m.SizeDecimals,
		MarkPrice:      m.MarkPrice.String(),
	}
}

func toMap(value any) map[string]any {
	var out map[string]any
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func toMapSlice[T any](items []T) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toMap(item))
	}
	return out
}
