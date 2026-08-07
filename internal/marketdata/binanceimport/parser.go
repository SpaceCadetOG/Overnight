package binanceimport

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// Stats summarizes the Binance CSV import.
type Stats struct {
	RowsRead      int
	CandlesParsed int
	MalformedRows int
	DuplicateRows int
}

// ReadCandlesCSV reads a headerless Binance kline CSV.
//
// Expected columns:
//
//	0 open time
//	1 open
//	2 high
//	3 low
//	4 close
//	5 volume
//	6 close time
//	7 quote volume
//	8 trade count
//	9 taker-buy base volume
//
// 10 taker-buy quote volume
// 11 ignore
//
// Binance Vision files may contain timestamps in either milliseconds
// or microseconds. Both are normalized automatically.
func ReadCandlesCSV(path string) ([]models.Candle, Stats, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, Stats{}, fmt.Errorf("open Binance CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false

	candles := make([]models.Candle, 0, 2_500_000)
	stats := Stats{}

	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}

		stats.RowsRead++

		if readErr != nil {
			stats.MalformedRows++
			continue
		}

		if isBlankRow(row) {
			continue
		}

		candle, parseErr := parseRow(row)
		if parseErr != nil {
			stats.MalformedRows++

			if stats.MalformedRows <= 10 {
				fmt.Fprintf(
					os.Stderr,
					"warning: skipped malformed Binance row %d: %v\n",
					stats.RowsRead,
					parseErr,
				)
			}

			continue
		}

		if validateErr := candle.Validate(); validateErr != nil {
			stats.MalformedRows++

			if stats.MalformedRows <= 10 {
				fmt.Fprintf(
					os.Stderr,
					"warning: skipped invalid Binance row %d: %v\n",
					stats.RowsRead,
					validateErr,
				)
			}

			continue
		}

		candles = append(candles, candle)
	}

	if len(candles) == 0 {
		return nil, stats, fmt.Errorf("Binance CSV contained no valid candles")
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})

	candles, duplicates := deduplicate(candles)
	stats.DuplicateRows = duplicates
	stats.CandlesParsed = len(candles)

	return candles, stats, nil
}

func parseRow(row []string) (models.Candle, error) {
	if len(row) < 7 {
		return models.Candle{}, fmt.Errorf(
			"expected at least 7 fields, received %d",
			len(row),
		)
	}

	openTime, err := parseBinanceTime(row[0])
	if err != nil {
		return models.Candle{}, fmt.Errorf("parse open time: %w", err)
	}

	closeTime, err := parseBinanceTime(row[6])
	if err != nil {
		return models.Candle{}, fmt.Errorf("parse close time: %w", err)
	}

	open, err := parseFloat(row[1], "open")
	if err != nil {
		return models.Candle{}, err
	}

	high, err := parseFloat(row[2], "high")
	if err != nil {
		return models.Candle{}, err
	}

	low, err := parseFloat(row[3], "low")
	if err != nil {
		return models.Candle{}, err
	}

	closePrice, err := parseFloat(row[4], "close")
	if err != nil {
		return models.Candle{}, err
	}

	volume, err := parseFloat(row[5], "volume")
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

func parseBinanceTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)

	timestamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"parse timestamp %q: %w",
			value,
			err,
		)
	}

	switch {
	case timestamp >= 1_000_000_000_000_000:
		// Binance Vision microseconds.
		return time.UnixMicro(timestamp), nil

	case timestamp >= 1_000_000_000_000:
		// Milliseconds.
		return time.UnixMilli(timestamp), nil

	case timestamp >= 1_000_000_000:
		// Seconds, supported defensively.
		return time.Unix(timestamp, 0), nil

	default:
		return time.Time{}, fmt.Errorf(
			"unsupported timestamp precision: %d",
			timestamp,
		)
	}
}

func parseFloat(value string, field string) (float64, error) {
	value = strings.TrimSpace(value)

	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"parse %s value %q: %w",
			field,
			value,
			err,
		)
	}

	return number, nil
}

func isBlankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}

	return true
}

func deduplicate(candles []models.Candle) ([]models.Candle, int) {
	if len(candles) == 0 {
		return candles, 0
	}

	result := make([]models.Candle, 0, len(candles))
	duplicates := 0

	var previousTimestamp int64 = -1

	for _, candle := range candles {
		timestamp := candle.OpenTime.UnixMilli()

		if timestamp == previousTimestamp {
			duplicates++
			continue
		}

		result = append(result, candle)
		previousTimestamp = timestamp
	}

	return result, duplicates
}
