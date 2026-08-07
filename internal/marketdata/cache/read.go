package cache

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// ReadCandlesCSV reads candles previously written by WriteCandlesCSV.
func ReadCandlesCSV(path string) ([]models.Candle, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open candle CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	if len(header) < 7 {
		return nil, fmt.Errorf(
			"expected at least 7 CSV columns, got %d",
			len(header),
		)
	}

	candles := make([]models.Candle, 0)

	rowNumber := 1

	for {
		rowNumber++

		row, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf(
				"read CSV row %d: %w",
				rowNumber,
				err,
			)
		}

		candle, err := parseCandleRow(row)
		if err != nil {
			return nil, fmt.Errorf(
				"parse CSV row %d: %w",
				rowNumber,
				err,
			)
		}

		if err := candle.Validate(); err != nil {
			return nil, fmt.Errorf(
				"validate CSV row %d: %w",
				rowNumber,
				err,
			)
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

func parseCandleRow(row []string) (models.Candle, error) {
	if len(row) < 7 {
		return models.Candle{}, fmt.Errorf(
			"expected at least 7 columns, got %d",
			len(row),
		)
	}

	openTime, err := time.Parse(time.RFC3339Nano, row[0])
	if err != nil {
		return models.Candle{}, fmt.Errorf("open time: %w", err)
	}

	closeTime, err := time.Parse(time.RFC3339Nano, row[1])
	if err != nil {
		return models.Candle{}, fmt.Errorf("close time: %w", err)
	}

	open, err := parseCSVFloat(row[2], "open")
	if err != nil {
		return models.Candle{}, err
	}

	high, err := parseCSVFloat(row[3], "high")
	if err != nil {
		return models.Candle{}, err
	}

	low, err := parseCSVFloat(row[4], "low")
	if err != nil {
		return models.Candle{}, err
	}

	closePrice, err := parseCSVFloat(row[5], "close")
	if err != nil {
		return models.Candle{}, err
	}

	volume, err := parseCSVFloat(row[6], "volume")
	if err != nil {
		return models.Candle{}, err
	}

	return models.Candle{
		OpenTime:  openTime.UTC(),
		CloseTime: closeTime.UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
	}, nil
}

func parseCSVFloat(value string, field string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s value %q: %w",
			field,
			value,
			err,
		)
	}

	return number, nil
}
