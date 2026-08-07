package liquidity

import "github.com/ogtrading/overnight-strategy/internal/models"

// ClassifyEvent labels the most recent interaction with mapped liquidity.
// Recency prevents an old run through a minor level from hiding the grab or
// sweep immediately preceding entry.
func ClassifyEvent(levels []Level, candles []models.Candle) Event {
	event, latestIndex := EventNone, -1
	for _, level := range levels {
		for i, candle := range candles {
			penetrated := level.Side == BuySide && candle.High > level.Price || level.Side == SellSide && candle.Low < level.Price
			if !penetrated {
				continue
			}
			candidate := EventGrab
			closedBeyond := level.Side == BuySide && candle.Close > level.Price || level.Side == SellSide && candle.Close < level.Price
			if closedBeyond && i+1 < len(candles) {
				nextBeyond := level.Side == BuySide && candles[i+1].Close > level.Price || level.Side == SellSide && candles[i+1].Close < level.Price
				if nextBeyond {
					candidate = EventRun
				} else {
					candidate = EventSweep
				}
			} else if closedBeyond {
				candidate = EventRun
			}
			if i >= latestIndex {
				latestIndex, event = i, candidate
			}
		}
	}
	return event
}

// HasConsumedLiquidity checks each pool independently. A grab or sweep means
// that pool was consumed; an unrelated run through another level does not
// erase that event.
func HasConsumedLiquidity(levels []Level, candles []models.Candle) bool {
	for _, level := range levels {
		event := ClassifyEvent([]Level{level}, candles)
		if event == EventGrab || event == EventSweep {
			return true
		}
	}
	return false
}
