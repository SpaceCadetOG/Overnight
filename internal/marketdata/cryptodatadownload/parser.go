package cryptodatadownload

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

// ReadCandlesCSV parses a CryptoDataDownload OHLCV CSV.
//
// The parser is intentionally tolerant because files can differ slightly
// between exchanges. It:
//   - skips promotional/comment rows before the header
//   - recognizes common timestamp and OHLCV column aliases
//   - supports Unix seconds, milliseconds, and formatted dates
//   - sorts output oldest to newest
//   - removes duplicate candle timestamps
func ReadCandlesCSV(path string) ([]models.Candle, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CryptoDataDownload CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	reader.ReuseRecord = false

	header, err := findHeader(reader)
	if err != nil {
		return nil, err
	}

	columns := indexColumns(header)

	if err := validateRequiredColumns(columns); err != nil {
		return nil, err
	}

	candles := make([]models.Candle, 0, 100_000)
	rowNumber := 1

	for {
		rowNumber++

		row, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read row %d: %w", rowNumber, err)
		}

		if isBlankRow(row) {
			continue
		}

		candle, err := parseRow(row, columns)
		if err != nil {
			return nil, fmt.Errorf("parse row %d: %w", rowNumber, err)
		}

		if err := candle.Validate(); err != nil {
			return nil, fmt.Errorf("validate row %d: %w", rowNumber, err)
		}

		candles = append(candles, candle)
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})

	return deduplicate(candles), nil
}

type columnIndexes struct {
	timestamp int
	date      int
	open      int
	high      int
	low       int
	close     int
	volume    int
}

func findHeader(reader *csv.Reader) ([]string, error) {
	for rowNumber := 1; rowNumber <= 25; rowNumber++ {
		row, err := reader.Read()
		if err == io.EOF {
			return nil, fmt.Errorf("CSV ended before a valid header was found")
		}

		if err != nil {
			return nil, fmt.Errorf("read potential header row %d: %w", rowNumber, err)
		}

		normalized := make([]string, len(row))
		for index, value := range row {
			normalized[index] = normalizeHeader(value)
		}

		hasOpen := contains(normalized, "open")
		hasHigh := contains(normalized, "high")
		hasLow := contains(normalized, "low")
		hasClose := contains(normalized, "close")

		if hasOpen && hasHigh && hasLow && hasClose {
			return row, nil
		}
	}

	return nil, fmt.Errorf("no OHLC header found in first 25 rows")
}

func indexColumns(header []string) columnIndexes {
	indexes := columnIndexes{
		timestamp: -1,
		date:      -1,
		open:      -1,
		high:      -1,
		low:       -1,
		close:     -1,
		volume:    -1,
	}

	for index, raw := range header {
		name := normalizeHeader(raw)

		switch {
		case name == "unix",
			name == "timestamp",
			name == "time",
			name == "epoch":
			if indexes.timestamp == -1 {
				indexes.timestamp = index
			}

		case name == "date",
			name == "datetime":
			if indexes.date == -1 {
				indexes.date = index
			}

		case name == "open":
			indexes.open = index

		case name == "high":
			indexes.high = index

		case name == "low":
			indexes.low = index

		case name == "close":
			indexes.close = index

		case isPreferredVolumeHeader(name):
			if indexes.volume == -1 || strings.Contains(name, "btc") {
				indexes.volume = index
			}
		}
	}

	return indexes
}

func validateRequiredColumns(columns columnIndexes) error {
	if columns.timestamp == -1 && columns.date == -1 {
		return fmt.Errorf("CSV must contain a Unix timestamp or date column")
	}

	required := map[string]int{
		"open":   columns.open,
		"high":   columns.high,
		"low":    columns.low,
		"close":  columns.close,
		"volume": columns.volume,
	}

	for name, index := range required {
		if index == -1 {
			return fmt.Errorf("required column %q was not found", name)
		}
	}

	return nil
}

func parseRow(
	row []string,
	columns columnIndexes,
) (models.Candle, error) {
	openTime, err := parseTimeColumn(row, columns)
	if err != nil {
		return models.Candle{}, err
	}

	open, err := parseFloatAt(row, columns.open, "open")
	if err != nil {
		return models.Candle{}, err
	}

	high, err := parseFloatAt(row, columns.high, "high")
	if err != nil {
		return models.Candle{}, err
	}

	low, err := parseFloatAt(row, columns.low, "low")
	if err != nil {
		return models.Candle{}, err
	}

	closePrice, err := parseFloatAt(row, columns.close, "close")
	if err != nil {
		return models.Candle{}, err
	}

	volume, err := parseFloatAt(row, columns.volume, "volume")
	if err != nil {
		return models.Candle{}, err
	}

	return models.Candle{
		OpenTime:  openTime.UTC(),
		CloseTime: openTime.Add(time.Minute - time.Millisecond).UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
	}, nil
}

func parseTimeColumn(
	row []string,
	columns columnIndexes,
) (time.Time, error) {
	if columns.timestamp >= 0 {
		value, err := valueAt(row, columns.timestamp, "timestamp")
		if err != nil {
			return time.Time{}, err
		}

		if parsed, ok := parseUnix(value); ok {
			return parsed.UTC(), nil
		}
	}

	if columns.date >= 0 {
		value, err := valueAt(row, columns.date, "date")
		if err != nil {
			return time.Time{}, err
		}

		parsed, err := parseFormattedDate(value)
		if err != nil {
			return time.Time{}, err
		}

		return parsed.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("could not parse candle timestamp")
}

func parseUnix(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)

	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, false
	}

	switch {
	case number > 100_000_000_000_000:
		return time.UnixMicro(number), true

	case number > 100_000_000_000:
		return time.UnixMilli(number), true

	default:
		return time.Unix(number, 0), true
	}
}

func parseFormattedDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
		time.RFC3339Nano,
		"01/02/2006 15:04",
		"01/02/2006 15:04:05",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date value %q", value)
}

func parseFloatAt(
	row []string,
	index int,
	field string,
) (float64, error) {
	value, err := valueAt(row, index, field)
	if err != nil {
		return 0, err
	}

	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", "")

	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s value %q: %w", field, value, err)
	}

	return number, nil
}

func valueAt(row []string, index int, field string) (string, error) {
	if index < 0 || index >= len(row) {
		return "", fmt.Errorf(
			"%s column index %d exceeds row length %d",
			field,
			index,
			len(row),
		)
	}

	return row[index], nil
}

func isPreferredVolumeHeader(name string) bool {
	if name == "volume" {
		return true
	}

	return strings.HasPrefix(name, "volume") &&
		!strings.Contains(name, "usd") &&
		!strings.Contains(name, "quote")
}

func normalizeHeader(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.Join(strings.Fields(value), " ")

	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}

func isBlankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}

	return true
}

func deduplicate(candles []models.Candle) []models.Candle {
	seen := make(map[int64]struct{}, len(candles))
	result := make([]models.Candle, 0, len(candles))

	for _, candle := range candles {
		key := candle.OpenTime.UnixMilli()

		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		result = append(result, candle)
	}

	return result
}
