package lighter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Position struct {
	Symbol           string
	Side             string
	Size             float64
	EntryPrice       float64
	UnrealizedPnL    float64
	LiquidationPrice float64
}

func (c *Client) GetPosition(accountIndex int64, symbol string) (*Position, error) {

	q := map[string]string{
		"by":    "index",
		"value": fmt.Sprintf("%d", accountIndex),
	}

	b, err := c.doGET("/api/v1/account", q)

	if err != nil {
		return nil, err
	}

	var resp struct {
		Code     int `json:"code"`
		Accounts []struct {
			Positions []struct {
				Symbol        string `json:"symbol"`
				Position      string `json:"position"`
				AvgEntryPrice string `json:"avg_entry_price"`
				UnrealizedPnL string `json:"unrealized_pnl"`
				Sign          int    `json:"sign"`
			} `json:"positions"`
		} `json:"accounts"`
	}

	err = json.Unmarshal(b, &resp)

	if err != nil {
		return nil, err
	}

	for _, p := range resp.Accounts[0].Positions {

		if p.Symbol != symbol {
			continue
		}

		size, _ := strconv.ParseFloat(p.Position, 64)
		entry, _ := strconv.ParseFloat(p.AvgEntryPrice, 64)
		pnl, _ := strconv.ParseFloat(p.UnrealizedPnL, 64)

		side := "FLAT"

		if size > 0 {
			if p.Sign == 1 {
				side = "LONG"
			} else {
				side = "SHORT"
			}
		}

		return &Position{
			Symbol:        p.Symbol,
			Side:          side,
			Size:          size,
			EntryPrice:    entry,
			UnrealizedPnL: pnl,
		}, nil
	}

	return nil, fmt.Errorf("position not found")
}
