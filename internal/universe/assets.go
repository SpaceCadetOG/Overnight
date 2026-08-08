package universe

import "fmt"

type AssetClass string
type Classification string

const (
	Crypto AssetClass = "CRYPTO"
	Metals AssetClass = "METALS"

	LiveExecution Classification = "LIVE_EXECUTION"
	Research      Classification = "RESEARCH"
)

type Asset struct {
	Symbol         string         `json:"symbol"`
	ExchangeSymbol string         `json:"exchange_symbol"`
	Class          AssetClass     `json:"asset_class"`
	Exchange       string         `json:"exchange"`
	Classification Classification `json:"classification"`
	Tradable       bool           `json:"tradable"`
	ResearchOnly   bool           `json:"research_only"`
}

var assets = []Asset{
	{Symbol: "BTC", Class: Crypto, Exchange: "lighter", Classification: LiveExecution, Tradable: true},
	{Symbol: "ETH", Class: Crypto, Exchange: "lighter", Classification: LiveExecution, Tradable: true},

	// Research assets receive the complete pipeline in paper mode but never route
	// to the funded executor.
	{Symbol: "SOL", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "HYPE", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "LIT", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "XAU", Class: Metals, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "XAG", Class: Metals, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "LINK", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "AAVE", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "UNI", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "ZEC", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
	{Symbol: "BNB", Class: Crypto, Exchange: "lighter", Classification: Research, ResearchOnly: true},
}

// MarketSymbol is the exchange-native market name. Symbol remains the stable
// research/reporting identity so exchange aliases never leak into experiments.
func (a Asset) MarketSymbol() string {
	if a.ExchangeSymbol != "" {
		return a.ExchangeSymbol
	}
	return a.Symbol
}

func All() []Asset { return append([]Asset(nil), assets...) }

func Live() []Asset { return selectAssets(func(a Asset) bool { return a.Tradable }) }

func Observed() []Asset { return selectAssets(func(a Asset) bool { return a.ResearchOnly }) }

func Find(symbol string) (Asset, bool) {
	for _, asset := range assets {
		if asset.Symbol == symbol {
			return asset, true
		}
	}
	return Asset{}, false
}

func RequireTradable(symbol string) (Asset, error) {
	asset, ok := Find(symbol)
	if !ok {
		return Asset{}, fmt.Errorf("asset %s is not registered", symbol)
	}
	if !asset.Tradable || asset.ResearchOnly {
		return Asset{}, fmt.Errorf("asset %s has no execution authority (%s)", symbol, asset.Classification)
	}
	return asset, nil
}

func selectAssets(keep func(Asset) bool) []Asset {
	out := make([]Asset, 0, len(assets))
	for _, asset := range assets {
		if keep(asset) {
			out = append(out, asset)
		}
	}
	return out
}
