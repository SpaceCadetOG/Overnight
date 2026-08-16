package lighteradapter

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"
)

type Number string

func (n *Number) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0, bytes.Equal(data, []byte("null")):
		*n = ""
		return nil
	case data[0] == '"':
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*n = Number(value)
		return nil
	default:
		*n = Number(string(data))
		return nil
	}
}

func (n Number) String() string {
	return string(n)
}

func (n Number) Float64() (float64, error) {
	if n == "" {
		return 0, nil
	}
	return strconv.ParseFloat(string(n), 64)
}

type Freshness string

const (
	FreshnessFresh  Freshness = "fresh"
	FreshnessStale  Freshness = "stale"
	FreshnessFailed Freshness = "failed"
)

type Market struct {
	Symbol         string `json:"symbol"`
	MarketID       int16  `json:"market_id"`
	Status         string `json:"status"`
	MarketType     string `json:"market_type"`
	OpenInterest   Number `json:"open_interest"`
	MarkPrice      Number `json:"mark_price"`
	MinBaseAmount  Number `json:"min_base_amount"`
	MinQuoteAmount Number `json:"min_quote_amount"`
	PriceDecimals  int    `json:"supported_price_decimals"`
	SizeDecimals   int    `json:"supported_size_decimals"`
	QuoteDecimals  int    `json:"supported_quote_decimals"`
}

type ExchangeStat struct {
	Symbol                string  `json:"symbol"`
	LastTradePrice        float64 `json:"last_trade_price"`
	DailyPriceChange      float64 `json:"daily_price_change"`
	DailyBaseTokenVolume  float64 `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume float64 `json:"daily_quote_token_volume"`
}

type FundingRate struct {
	MarketID int     `json:"market_id"`
	Exchange string  `json:"exchange"`
	Symbol   string  `json:"symbol"`
	Rate     float64 `json:"rate"`
}

type Position struct {
	MarketID               int    `json:"market_id"`
	Symbol                 string `json:"symbol"`
	InitialMarginFraction  string `json:"initial_margin_fraction"`
	OpenOrderCount         int    `json:"open_order_count"`
	PendingOrderCount      int    `json:"pending_order_count"`
	PositionTiedOrderCount int    `json:"position_tied_order_count"`
	Sign                   int    `json:"sign"`
	Position               string `json:"position"`
	AvgEntryPrice          string `json:"avg_entry_price"`
	PositionValue          string `json:"position_value"`
	UnrealizedPnl          string `json:"unrealized_pnl"`
	RealizedPnl            string `json:"realized_pnl"`
	LiquidationPrice       string `json:"liquidation_price"`
	MarginMode             int    `json:"margin_mode"`
	AllocatedMargin        string `json:"allocated_margin"`
}

type Account struct {
	Code                     int        `json:"code"`
	AccountType              int        `json:"account_type"`
	Index                    int64      `json:"index"`
	L1Address                string     `json:"l1_address"`
	CancelAllTime            int64      `json:"cancel_all_time"`
	TotalOrderCount          int        `json:"total_order_count"`
	TotalIsolatedOrderCount  int        `json:"total_isolated_order_count"`
	PendingOrderCount        int        `json:"pending_order_count"`
	AvailableBalance         string     `json:"available_balance"`
	Status                   int        `json:"status"`
	Collateral               string     `json:"collateral"`
	AccountIndex             int64      `json:"account_index"`
	Name                     string     `json:"name"`
	Description              string     `json:"description"`
	CanInvite                bool       `json:"can_invite"`
	ReferralPointsPercentage string     `json:"referral_points_percentage"`
	Positions                []Position `json:"positions"`
	TotalAssetValue          string     `json:"total_asset_value"`
	CrossAssetValue          string     `json:"cross_asset_value"`
	Shares                   []any      `json:"shares"`
	AccountTradingMode       int        `json:"account_trading_mode,omitempty"`
}

type Order struct {
	OrderIndex          int64  `json:"order_index"`
	ClientOrderIndex    int64  `json:"client_order_index"`
	OrderID             string `json:"order_id"`
	ClientOrderID       string `json:"client_order_id"`
	MarketIndex         int    `json:"market_index"`
	OwnerAccountIndex   int64  `json:"owner_account_index"`
	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	FilledBaseAmount    string `json:"filled_base_amount"`
	FilledQuoteAmount   string `json:"filled_quote_amount"`
	Price               string `json:"price"`
	Type                string `json:"type"`
	TimeInForce         string `json:"time_in_force"`
	Status              string `json:"status"`
	ReduceOnly          bool   `json:"reduce_only"`
	TriggerPrice        string `json:"trigger_price"`
	IsAsk               bool   `json:"is_ask"`
	Nonce               uint64 `json:"nonce"`
}

type TradeFill struct {
	TradeID                          int64  `json:"trade_id"`
	TradeIDStr                       string `json:"trade_id_str"`
	TxHash                           string `json:"tx_hash"`
	Type                             string `json:"type"`
	MarketID                         int    `json:"market_id"`
	Size                             string `json:"size"`
	Price                            string `json:"price"`
	USDAmount                        string `json:"usd_amount"`
	AskID                            int64  `json:"ask_id"`
	AskIDStr                         string `json:"ask_id_str"`
	BidID                            int64  `json:"bid_id"`
	BidIDStr                         string `json:"bid_id_str"`
	AskClientID                      int64  `json:"ask_client_id"`
	AskClientIDStr                   string `json:"ask_client_id_str"`
	BidClientID                      int64  `json:"bid_client_id"`
	BidClientIDStr                   string `json:"bid_client_id_str"`
	AskAccountID                     int64  `json:"ask_account_id"`
	BidAccountID                     int64  `json:"bid_account_id"`
	IsMakerAsk                       bool   `json:"is_maker_ask"`
	BlockHeight                      int64  `json:"block_height"`
	Timestamp                        int64  `json:"timestamp"`
	TakerFee                         int64  `json:"taker_fee"`
	TakerPositionSizeBefore          string `json:"taker_position_size_before"`
	TakerEntryQuoteBefore            string `json:"taker_entry_quote_before"`
	TakerInitialMarginFractionBefore int    `json:"taker_initial_margin_fraction_before"`
	TakerPositionSignChanged         bool   `json:"taker_position_sign_changed"`
	MakerPositionSizeBefore          string `json:"maker_position_size_before"`
	MakerEntryQuoteBefore            string `json:"maker_entry_quote_before"`
	MakerPositionSignChanged         bool   `json:"maker_position_sign_changed"`
	TransactionTime                  int64  `json:"transaction_time"`
	AskAccountPnL                    string `json:"ask_account_pnl"`
	TakerAllocatedMarginUSDCBefore   int64  `json:"taker_allocated_margin_usdc_before"`
	AskOrderVersion                  int    `json:"ask_order_version"`
	BidOrderVersion                  int    `json:"bid_order_version"`
}

type FundingRecord struct {
	Timestamp    int64  `json:"timestamp"`
	MarketID     int    `json:"market_id"`
	FundingID    int64  `json:"funding_id"`
	Change       string `json:"change"`
	Rate         string `json:"rate"`
	PositionSize string `json:"position_size"`
	PositionSide string `json:"position_side"`
	Discount     string `json:"discount"`
}

type MarketsResult struct {
	Code          int       `json:"code"`
	Items         []Market  `json:"items"`
	Freshness     Freshness `json:"freshness"`
	RetrievedAt   time.Time `json:"retrieved_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	Authoritative bool      `json:"authoritative"`
}

type ExchangeStatsResult struct {
	Code          int            `json:"code"`
	Items         []ExchangeStat `json:"items"`
	Freshness     Freshness      `json:"freshness"`
	RetrievedAt   time.Time      `json:"retrieved_at,omitempty"`
	Error         string         `json:"error,omitempty"`
	Authoritative bool           `json:"authoritative"`
}

type FundingRatesResult struct {
	Code          int           `json:"code"`
	Items         []FundingRate `json:"items"`
	Freshness     Freshness     `json:"freshness"`
	RetrievedAt   time.Time     `json:"retrieved_at,omitempty"`
	Error         string        `json:"error,omitempty"`
	Authoritative bool          `json:"authoritative"`
}

type AccountsResult struct {
	Code          int       `json:"code"`
	Total         int       `json:"total"`
	Items         []Account `json:"items"`
	Freshness     Freshness `json:"freshness"`
	RetrievedAt   time.Time `json:"retrieved_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	Authoritative bool      `json:"authoritative"`
}

type ActiveOrdersResult struct {
	Code          int       `json:"code"`
	NextCursor    string    `json:"next_cursor,omitempty"`
	Items         []Order   `json:"items"`
	Freshness     Freshness `json:"freshness"`
	RetrievedAt   time.Time `json:"retrieved_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	Authoritative bool      `json:"authoritative"`
}

type HistoricalOrdersResult struct {
	Code          int       `json:"code"`
	NextCursor    string    `json:"next_cursor,omitempty"`
	Items         []Order   `json:"items"`
	Freshness     Freshness `json:"freshness"`
	RetrievedAt   time.Time `json:"retrieved_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	Authoritative bool      `json:"authoritative"`
}

type FillsResult struct {
	Code          int         `json:"code"`
	NextCursor    string      `json:"next_cursor,omitempty"`
	Items         []TradeFill `json:"items"`
	Freshness     Freshness   `json:"freshness"`
	RetrievedAt   time.Time   `json:"retrieved_at,omitempty"`
	Error         string      `json:"error,omitempty"`
	Authoritative bool        `json:"authoritative"`
}

type FundingResult struct {
	Code          int             `json:"code"`
	NextCursor    string          `json:"next_cursor,omitempty"`
	Items         []FundingRecord `json:"items"`
	Freshness     Freshness       `json:"freshness"`
	RetrievedAt   time.Time       `json:"retrieved_at,omitempty"`
	Error         string          `json:"error,omitempty"`
	Authoritative bool            `json:"authoritative"`
}

type PrivateWSCheckResult struct {
	Channel       string    `json:"channel"`
	Freshness     Freshness `json:"freshness"`
	RetrievedAt   time.Time `json:"retrieved_at,omitempty"`
	Error         string    `json:"error,omitempty"`
	Authoritative bool      `json:"authoritative"`
}

type AuthTokenProvider interface {
	AuthToken(expiresAt time.Time) (string, error)
}

type AuthTokenFunc func(expiresAt time.Time) (string, error)

func (fn AuthTokenFunc) AuthToken(expiresAt time.Time) (string, error) {
	return fn(expiresAt)
}

type PublicReader interface {
	Markets(ctx context.Context) (MarketsResult, error)
	ExchangeStats(ctx context.Context) (ExchangeStatsResult, error)
	FundingRates(ctx context.Context) (FundingRatesResult, error)
}

type AccountReader interface {
	AccountsByL1(ctx context.Context, address string) (AccountsResult, error)
	AccountByIndex(ctx context.Context, accountIndex int64) (AccountsResult, error)
	ActiveOrders(ctx context.Context, accountIndex int64, marketID int) (ActiveOrdersResult, error)
	HistoricalOrders(ctx context.Context, accountIndex int64, marketID int, limit int) (HistoricalOrdersResult, error)
	Fills(ctx context.Context, accountIndex int64, limit int) (FillsResult, error)
	Funding(ctx context.Context, accountIndex int64, limit int, side string) (FundingResult, error)
}

type HealthReader interface {
	CheckPrivateWebSocket(ctx context.Context, wsURL string, accountIndex int64) (PrivateWSCheckResult, error)
}
