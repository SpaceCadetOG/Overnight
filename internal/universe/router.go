package universe

import "fmt"

type Destination string

const (
	LiveExecutor  Destination = "LIVE_EXECUTOR"
	PaperExecutor Destination = "PAPER_EXECUTOR"
)

func Resolve(symbol string) (Destination, error) {
	asset, ok := Find(symbol)
	if !ok {
		return "", fmt.Errorf("asset not registered: %s", symbol)
	}
	if asset.Tradable && !asset.ResearchOnly {
		return LiveExecutor, nil
	}
	return PaperExecutor, nil
}
