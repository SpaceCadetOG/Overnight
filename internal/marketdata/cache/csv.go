package cache

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// WriteCandlesCSV writes candles to a local CSV file.
func WriteCandlesCSV(path string, candles []models.Candle) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create candle cache: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"open_time",
		"close_time",
		"open",
		"high",
		"low",
		"close",
		"volume",
	}

	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	for _, candle := range candles {
		row := []string{
			candle.OpenTime.UTC().Format(time.RFC3339Nano),
			candle.CloseTime.UTC().Format(time.RFC3339Nano),
			strconv.FormatFloat(candle.Open, 'f', -1, 64),
			strconv.FormatFloat(candle.High, 'f', -1, 64),
			strconv.FormatFloat(candle.Low, 'f', -1, 64),
			strconv.FormatFloat(candle.Close, 'f', -1, 64),
			strconv.FormatFloat(candle.Volume, 'f', -1, 64),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write candle row: %w", err)
		}
	}

	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush candle CSV: %w", err)
	}

	return nil
}
