package lighter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	lighterclient "github.com/elliottech/lighter-go/client"
	lighterhttp "github.com/elliottech/lighter-go/client/http"
	"github.com/elliottech/lighter-go/types"
)

type Order struct {
	OrderIndex        int64  `json:"order_index"`
	ClientOrderIndex  int64  `json:"client_order_index"`
	OrderID           string `json:"order_id"`
	ClientOrderID     string `json:"client_order_id"`
	MarketIndex       int16  `json:"market_index"`
	OwnerAccountIndex int64  `json:"owner_account_index"`

	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	FilledBaseAmount    string `json:"filled_base_amount"`
	FilledQuoteAmount   string `json:"filled_quote_amount"`

	Price string `json:"price"`
	Nonce int64  `json:"nonce"`

	IsAsk bool   `json:"is_ask"`
	Side  string `json:"side"`

	BaseSize  int64  `json:"base_size"`
	BasePrice uint32 `json:"base_price"`

	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	ReduceOnly  bool   `json:"reduce_only"`

	Status string `json:"status"`

	TriggerPrice  string `json:"trigger_price"`
	OrderExpiry   int64  `json:"order_expiry"`
	TriggerStatus string `json:"trigger_status"`
	TriggerTime   int64  `json:"trigger_time"`

	ParentOrderIndex  int64  `json:"parent_order_index"`
	ParentOrderID     string `json:"parent_order_id"`
	ToTriggerOrderID0 string `json:"to_trigger_order_id_0"`
	ToTriggerOrderID1 string `json:"to_trigger_order_id_1"`
	ToCancelOrderID0  string `json:"to_cancel_order_id_0"`

	BlockHeight     int64 `json:"block_height"`
	Timestamp       int64 `json:"timestamp"`
	CreatedAt       int64 `json:"created_at"`
	UpdatedAt       int64 `json:"updated_at"`
	TransactionTime int64 `json:"transaction_time"`
	OrderVersion    int64 `json:"order_version"`
}

type activeOrdersResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Orders  []Order `json:"orders"`
}

type nextNonceResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Nonce   int64  `json:"nonce"`
}

type sendTxResponse struct {
	Code                     int    `json:"code"`
	Message                  string `json:"message"`
	TxHash                   string `json:"tx_hash"`
	PredictedExecutionTimeMS int64  `json:"predicted_execution_time_ms"`
	VolumeQuotaRemaining     int64  `json:"volume_quota_remaining"`
}

type Manager struct {
	BaseURL      string
	ChainID      uint32
	AccountIndex int64
	APIKeyIndex  uint8
	PrivateKey   string

	HTTPClient *http.Client
	TxClient   *lighterclient.TxClient

	// authTokenFunc is used by package tests to exercise authenticated REST
	// behavior without constructing a live signing client.
	authTokenFunc func() (string, error)
}

type Config struct {
	BaseURL      string
	ChainID      uint32
	AccountIndex int64
	APIKeyIndex  uint8
	PrivateKey   string
}

func NewManager(cfg Config) (*Manager, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("base URL is required")
	}

	if cfg.ChainID == 0 {
		return nil, errors.New("chain ID is required")
	}

	if cfg.AccountIndex <= 0 {
		return nil, errors.New("account index is required")
	}

	if cfg.PrivateKey == "" {
		return nil, errors.New("private key is required")
	}

	httpClient := lighterhttp.NewClient(cfg.BaseURL)

	txClient, err := lighterclient.CreateClient(
		httpClient,
		cfg.PrivateKey,
		cfg.ChainID,
		cfg.APIKeyIndex,
		cfg.AccountIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("create tx client: %w", err)
	}

	if err := txClient.Check(); err != nil {
		return nil, fmt.Errorf("tx client check: %w", err)
	}

	return &Manager{
		BaseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		ChainID:      cfg.ChainID,
		AccountIndex: cfg.AccountIndex,
		APIKeyIndex:  cfg.APIKeyIndex,
		PrivateKey:   cfg.PrivateKey,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		TxClient: txClient,
	}, nil
}

func (m *Manager) authToken() (string, error) {
	if m.authTokenFunc != nil {
		return m.authTokenFunc()
	}
	if m.TxClient == nil {
		return "", errors.New("tx client is not configured")
	}
	token, err := m.TxClient.GetAuthToken(time.Now().Add(time.Hour))
	if err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}

	return token, nil
}

func (m *Manager) ActiveOrders(
	ctx context.Context,
	marketID *int16,
) ([]Order, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(m.BaseURL + "/api/v1/accountActiveOrders")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("account_index", strconv.FormatInt(m.AccountIndex, 10))

	if marketID != nil {
		q.Set("market_id", strconv.FormatInt(int64(*marketID), 10))
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		u.String(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("Authorization", token)

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("active orders request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"active orders HTTP %s: %s",
			res.Status,
			string(body),
		)
	}

	var response activeOrdersResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode active orders: %w", err)
	}

	if response.Code != 200 {
		return nil, fmt.Errorf(
			"active orders API error code=%d message=%q",
			response.Code,
			response.Message,
		)
	}

	return response.Orders, nil
}

func (m *Manager) FindByClientOrderIndex(
	ctx context.Context,
	clientOrderIndex int64,
) (*Order, error) {
	orders, err := m.ActiveOrders(ctx, nil)
	if err != nil {
		return nil, err
	}

	for i := range orders {
		if orders[i].ClientOrderIndex == clientOrderIndex {
			return &orders[i], nil
		}
	}

	return nil, fmt.Errorf(
		"active order with client_order_index=%d not found",
		clientOrderIndex,
	)
}

func (m *Manager) NextNonce(ctx context.Context) (int64, error) {
	u, err := url.Parse(m.BaseURL + "/api/v1/nextNonce")
	if err != nil {
		return 0, err
	}

	q := u.Query()
	q.Set("account_index", strconv.FormatInt(m.AccountIndex, 10))
	q.Set("api_key_index", strconv.FormatUint(uint64(m.APIKeyIndex), 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		u.String(),
		nil,
	)
	if err != nil {
		return 0, err
	}

	req.Header.Set("accept", "application/json")

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("next nonce request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf(
			"next nonce HTTP %s: %s",
			res.Status,
			string(body),
		)
	}

	var response nextNonceResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("decode next nonce: %w", err)
	}

	if response.Code != 200 {
		return 0, fmt.Errorf(
			"next nonce API error code=%d message=%q",
			response.Code,
			response.Message,
		)
	}

	return response.Nonce, nil
}

func (m *Manager) sendTx(
	ctx context.Context,
	txType uint8,
	txInfo string,
) (*sendTxResponse, error) {
	form := url.Values{}
	form.Set("tx_type", strconv.Itoa(int(txType)))
	form.Set("tx_info", txInfo)
	form.Set("price_protection", "true")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		m.BaseURL+"/api/v1/sendTx",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set(
		"content-type",
		"application/x-www-form-urlencoded",
	)

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send tx request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"send tx HTTP %s: %s",
			res.Status,
			string(body),
		)
	}

	var response sendTxResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode send tx: %w", err)
	}

	if response.Code != 200 {
		return nil, fmt.Errorf(
			"send tx API error code=%d message=%q",
			response.Code,
			response.Message,
		)
	}

	return &response, nil
}

func (m *Manager) Cancel(
	ctx context.Context,
	order Order,
) (string, error) {
	// Always get a fresh nonce immediately before signing.
	nonce, err := m.NextNonce(ctx)
	if err != nil {
		return "", fmt.Errorf("cancel nonce: %w", err)
	}

	return m.cancelWithNonce(ctx, order, nonce)
}

func (m *Manager) cancelWithNonce(ctx context.Context, order Order, nonce int64) (string, error) {
	req := &types.CancelOrderTxReq{
		MarketIndex: order.MarketIndex,

		// IMPORTANT:
		// Cancel uses the exchange order_index,
		// NOT client_order_index.
		Index: order.OrderIndex,
	}

	opts := &types.TransactOpts{
		Nonce: &nonce,
	}

	tx, err := m.TxClient.GetCancelOrderTransaction(req, opts)
	if err != nil {
		return "", fmt.Errorf("build cancel transaction: %w", err)
	}

	if err := tx.Validate(); err != nil {
		return "", fmt.Errorf("validate cancel transaction: %w", err)
	}

	txInfo, err := tx.GetTxInfo()
	if err != nil {
		return "", fmt.Errorf("encode cancel transaction: %w", err)
	}

	response, err := m.sendTx(
		ctx,
		tx.GetTxType(),
		txInfo,
	)
	if err != nil {
		return "", err
	}

	return response.TxHash, nil
}

func (m *Manager) CancelByClientOrderIndex(
	ctx context.Context,
	clientOrderIndex int64,
) (string, error) {
	order, err := m.FindByClientOrderIndex(
		ctx,
		clientOrderIndex,
	)
	if err != nil {
		return "", err
	}

	return m.Cancel(ctx, *order)
}

func (m *Manager) WaitUntilRemoved(
	ctx context.Context,
	orderIndex int64,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"timeout waiting for order_index=%d to disappear",
				orderIndex,
			)
		}

		orders, err := m.ActiveOrders(ctx, nil)
		if err != nil {
			return err
		}

		found := false

		for _, order := range orders {
			if order.OrderIndex == orderIndex {
				found = true
				break
			}
		}

		if !found {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (m *Manager) CancelAndVerify(
	ctx context.Context,
	order Order,
) (string, error) {
	txHash, err := m.Cancel(ctx, order)
	if err != nil {
		return "", err
	}

	if err := m.WaitUntilRemoved(
		ctx,
		order.OrderIndex,
		10*time.Second,
	); err != nil {
		return txHash, fmt.Errorf(
			"cancel accepted but verification failed: %w",
			err,
		)
	}

	return txHash, nil
}

func (m *Manager) CancelAll(
	ctx context.Context,
) error {
	orders, err := m.ActiveOrders(ctx, nil)
	if err != nil {
		return err
	}

	if len(orders) == 0 {
		return nil
	}

	for _, order := range orders {
		_, err := m.CancelAndVerify(ctx, order)
		if err != nil {
			return fmt.Errorf(
				"cancel order_index=%d client_order_index=%d: %w",
				order.OrderIndex,
				order.ClientOrderIndex,
				err,
			)
		}
	}

	return nil
}
