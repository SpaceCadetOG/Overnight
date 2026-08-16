package lighter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type PositionSide string

const (
	PositionSideFlat  PositionSide = "FLAT"
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"
)

type Position struct {
	MarketID               int16  `json:"market_id"`
	Symbol                 string `json:"symbol"`
	InitialMarginFraction  string `json:"initial_margin_fraction"`
	OpenOrderCount         int64  `json:"open_order_count"`
	PendingOrderCount      int64  `json:"pending_order_count"`
	PositionTiedOrderCount int64  `json:"position_tied_order_count"`
	Sign                   int8   `json:"sign"`
	Size                   string `json:"position"`
	AverageEntryPrice      string `json:"avg_entry_price"`
	PositionValue          string `json:"position_value"`
	UnrealizedPnL          string `json:"unrealized_pnl"`
	RealizedPnL            string `json:"realized_pnl"`
	LiquidationPrice       string `json:"liquidation_price"`
	TotalFundingPaidOut    string `json:"total_funding_paid_out"`
	MarginMode             int    `json:"margin_mode"`
	AllocatedMargin        string `json:"allocated_margin"`
	TotalDiscount          string `json:"total_discount"`
}

type CanonicalPosition struct {
	MarketID               int16        `json:"market_id"`
	Symbol                 string       `json:"symbol"`
	Side                   PositionSide `json:"side"`
	Size                   string       `json:"size"`
	AverageEntryPrice      string       `json:"average_entry_price"`
	PositionValue          string       `json:"position_value"`
	UnrealizedPnL          string       `json:"unrealized_pnl"`
	RealizedPnL            string       `json:"realized_pnl"`
	LiquidationPrice       string       `json:"liquidation_price"`
	OpenOrderCount         int64        `json:"open_order_count"`
	PendingOrderCount      int64        `json:"pending_order_count"`
	PositionTiedOrderCount int64        `json:"position_tied_order_count"`
	MarginMode             int          `json:"margin_mode"`
	AllocatedMargin        string       `json:"allocated_margin"`
}

type PositionSnapshot struct {
	AccountIndex     int64               `json:"account_index"`
	Collateral       string              `json:"collateral"`
	AvailableBalance string              `json:"available_balance"`
	Positions        []CanonicalPosition `json:"positions"`
	TransactionTime  int64               `json:"transaction_time"`
}

type detailedAccount struct {
	AccountIndex     int64      `json:"account_index"`
	Index            int64      `json:"index"`
	Collateral       string     `json:"collateral"`
	AvailableBalance string     `json:"available_balance"`
	Positions        []Position `json:"positions"`
	TransactionTime  int64      `json:"transaction_time"`
}

type detailedAccountsResponse struct {
	Code       int               `json:"code"`
	Message    string            `json:"message"`
	Accounts   []detailedAccount `json:"accounts"`
	NextCursor string            `json:"next_cursor"`
}

func normalizePosition(position Position) (CanonicalPosition, error) {
	size, ok := decimal(position.Size)
	if !ok || size.Sign() < 0 {
		return CanonicalPosition{}, fmt.Errorf("position %s has invalid size %q", position.Symbol, position.Size)
	}

	side := PositionSideFlat
	if size.Sign() > 0 {
		switch position.Sign {
		case 1:
			side = PositionSideLong
		case -1:
			side = PositionSideShort
		default:
			return CanonicalPosition{}, fmt.Errorf("position %s has size %s with invalid sign %d", position.Symbol, position.Size, position.Sign)
		}
	}

	return CanonicalPosition{
		MarketID: position.MarketID, Symbol: normalizeSymbol(position.Symbol), Side: side, Size: position.Size,
		AverageEntryPrice: position.AverageEntryPrice, PositionValue: position.PositionValue,
		UnrealizedPnL: position.UnrealizedPnL, RealizedPnL: position.RealizedPnL,
		LiquidationPrice: position.LiquidationPrice, OpenOrderCount: position.OpenOrderCount,
		PendingOrderCount: position.PendingOrderCount, PositionTiedOrderCount: position.PositionTiedOrderCount,
		MarginMode: position.MarginMode, AllocatedMargin: position.AllocatedMargin,
	}, nil
}

// PositionSnapshot reads exchange-authoritative account and position state.
func (m *Manager) PositionSnapshot(ctx context.Context) (*PositionSnapshot, error) {
	u, err := url.Parse(m.BaseURL + "/api/v1/account")
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("by", "index")
	query.Set("value", strconv.FormatInt(m.AccountIndex, 10))
	query.Set("active_only", "true")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account positions request: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account positions HTTP %s: %s", res.Status, string(body))
	}

	var response detailedAccountsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode account positions: %w", err)
	}
	if response.Code != http.StatusOK {
		return nil, fmt.Errorf("account positions API error code=%d message=%q", response.Code, response.Message)
	}
	if len(response.Accounts) != 1 {
		return nil, fmt.Errorf("account positions returned %d accounts, expected 1", len(response.Accounts))
	}
	account := response.Accounts[0]
	accountIndex := account.AccountIndex
	if accountIndex == 0 {
		accountIndex = account.Index
	}
	if accountIndex != m.AccountIndex {
		return nil, fmt.Errorf("account positions returned account_index=%d, expected %d", accountIndex, m.AccountIndex)
	}

	positions := make([]CanonicalPosition, 0, len(account.Positions))
	for _, raw := range account.Positions {
		position, err := normalizePosition(raw)
		if err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return &PositionSnapshot{
		AccountIndex: accountIndex, Collateral: account.Collateral, AvailableBalance: account.AvailableBalance,
		Positions: positions, TransactionTime: account.TransactionTime,
	}, nil
}

func (s PositionSnapshot) Position(symbol string) (CanonicalPosition, bool) {
	wanted := normalizeSymbol(symbol)
	for _, position := range s.Positions {
		if normalizeSymbol(position.Symbol) == wanted {
			return position, true
		}
	}
	return CanonicalPosition{Symbol: wanted, Side: PositionSideFlat, Size: "0"}, false
}

type PositionExpectation struct {
	Symbol string       `json:"symbol"`
	Side   PositionSide `json:"side"`
	Size   string       `json:"size"`
}

type PositionDiscrepancyKind string

const (
	PositionUnexpected   PositionDiscrepancyKind = "UNEXPECTED_EXPOSURE"
	PositionMissing      PositionDiscrepancyKind = "MISSING_POSITION"
	PositionSideMismatch PositionDiscrepancyKind = "SIDE_MISMATCH"
	PositionSizeMismatch PositionDiscrepancyKind = "SIZE_MISMATCH"
)

type PositionDiscrepancy struct {
	Kind     PositionDiscrepancyKind `json:"kind"`
	Symbol   string                  `json:"symbol"`
	Expected *PositionExpectation    `json:"expected,omitempty"`
	Actual   *CanonicalPosition      `json:"actual,omitempty"`
}

func ComparePositions(snapshot PositionSnapshot, expected []PositionExpectation) ([]PositionDiscrepancy, error) {
	expectedBySymbol := make(map[string]PositionExpectation, len(expected))
	for _, item := range expected {
		item.Symbol = normalizeSymbol(item.Symbol)
		if item.Symbol == "" {
			return nil, fmt.Errorf("expected position symbol is required")
		}
		if item.Side != PositionSideFlat && item.Side != PositionSideLong && item.Side != PositionSideShort {
			return nil, fmt.Errorf("expected position %s has invalid side %q", item.Symbol, item.Side)
		}
		if _, duplicate := expectedBySymbol[item.Symbol]; duplicate {
			return nil, fmt.Errorf("duplicate expected position %s", item.Symbol)
		}
		expectedBySymbol[item.Symbol] = item
	}

	discrepancies := make([]PositionDiscrepancy, 0)
	seen := make(map[string]bool)
	for i := range snapshot.Positions {
		actual := &snapshot.Positions[i]
		symbol := normalizeSymbol(actual.Symbol)
		want, exists := expectedBySymbol[symbol]
		if !exists {
			if actual.Side != PositionSideFlat {
				discrepancies = append(discrepancies, PositionDiscrepancy{Kind: PositionUnexpected, Symbol: symbol, Actual: actual})
			}
			continue
		}
		seen[symbol] = true
		wantCopy := want
		if actual.Side != want.Side {
			discrepancies = append(discrepancies, PositionDiscrepancy{Kind: PositionSideMismatch, Symbol: symbol, Expected: &wantCopy, Actual: actual})
			continue
		}
		actualSize, actualOK := decimal(actual.Size)
		wantedSize, wantedOK := decimal(want.Size)
		if !actualOK || !wantedOK {
			return nil, fmt.Errorf("compare position %s: invalid size", symbol)
		}
		if actualSize.Cmp(wantedSize) != 0 {
			discrepancies = append(discrepancies, PositionDiscrepancy{Kind: PositionSizeMismatch, Symbol: symbol, Expected: &wantCopy, Actual: actual})
		}
	}

	for symbol, want := range expectedBySymbol {
		if seen[symbol] || want.Side == PositionSideFlat {
			continue
		}
		wantCopy := want
		discrepancies = append(discrepancies, PositionDiscrepancy{Kind: PositionMissing, Symbol: symbol, Expected: &wantCopy})
	}
	return discrepancies, nil
}

func PositionIsFlat(position CanonicalPosition) bool {
	return position.Side == PositionSideFlat || strings.TrimSpace(position.Size) == "" || strings.TrimSpace(position.Size) == "0"
}
