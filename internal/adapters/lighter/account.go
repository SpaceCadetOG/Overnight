package lighter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Account struct {
	AccountIndex int64
	Available    float64
	Collateral   float64
	TotalValue   float64
	Positions    []Position
}

type accountResponse struct {
	Code int `json:"code"`

	Accounts []struct {
		Index int64 `json:"index"`

		Available       string `json:"available_balance"`
		Collateral      string `json:"collateral"`
		TotalAssetValue string `json:"total_asset_value"`

		Positions []struct {
			Symbol           string `json:"symbol"`
			Position         string `json:"position"`
			AvgEntryPrice    string `json:"avg_entry_price"`
			UnrealizedPnL    string `json:"unrealized_pnl"`
			LiquidationPrice string `json:"liquidation_price"`
		} `json:"positions"`
	} `json:"accounts"`
}

func (c *Client) GetAccount(accountIndex int64) (*Account, error) {

	q := map[string]string{
		"by":    "index",
		"value": fmt.Sprintf("%d", accountIndex),
	}

	b, err := c.doGET(
		"/api/v1/account",
		q,
	)

	if err != nil {
		return nil, err
	}

	var out accountResponse

	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	if len(out.Accounts) == 0 {
		return nil, fmt.Errorf("account not found")
	}

	a := out.Accounts[0]

	available, _ := strconv.ParseFloat(a.Available, 64)
	collateral, _ := strconv.ParseFloat(a.Collateral, 64)
	total, _ := strconv.ParseFloat(a.TotalAssetValue, 64)

	positions := make([]Position, 0)

	for _, p := range a.Positions {

		size, _ := strconv.ParseFloat(p.Position, 64)
		entry, _ := strconv.ParseFloat(p.AvgEntryPrice, 64)
		pnl, _ := strconv.ParseFloat(p.UnrealizedPnL, 64)

		positions = append(positions, Position{
			Symbol:        p.Symbol,
			Size:          size,
			EntryPrice:    entry,
			UnrealizedPnL: pnl,
		})
	}

	return &Account{

		AccountIndex: a.Index,
		Available:    available,
		Collateral:   collateral,
		TotalValue:   total,
		Positions:    positions,
	}, nil
}
