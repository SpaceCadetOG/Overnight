package binanceimport

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseMillisecondTimestamp(t *testing.T) {
	got, err := parseBinanceTime("1640995200000")
	if err != nil {
		t.Fatalf("parse milliseconds: %v", err)
	}

	expected := time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)

	if !got.UTC().Equal(expected) {
		t.Fatalf("expected %s, received %s", expected, got.UTC())
	}
}

func TestParseMicrosecondTimestamp(t *testing.T) {
	got, err := parseBinanceTime("1764547200000000")
	if err != nil {
		t.Fatalf("parse microseconds: %v", err)
	}

	expected := time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC)

	if !got.UTC().Equal(expected) {
		t.Fatalf("expected %s, received %s", expected, got.UTC())
	}
}

func TestReadMixedTimestampCSV(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "mixed.csv")

	content := "" +
		"1640995200000,100,105,99,104,10,1640995259999,0,0,0,0,0\n" +
		"1764547200000000,200,205,199,204,20,1764547259999999,0,0,0,0,0\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test CSV: %v", err)
	}

	candles, stats, err := ReadCandlesCSV(path)
	if err != nil {
		t.Fatalf("read mixed CSV: %v", err)
	}

	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, received %d", len(candles))
	}

	if stats.MalformedRows != 0 {
		t.Fatalf("expected no malformed rows, received %d", stats.MalformedRows)
	}

	if !candles[0].OpenTime.Before(candles[1].OpenTime) {
		t.Fatal("candles were not sorted chronologically")
	}
}

func TestDuplicateTimestampRemoved(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "duplicates.csv")

	content := "" +
		"1640995200000,100,105,99,104,10,1640995259999,0,0,0,0,0\n" +
		"1640995200000,100,105,99,104,10,1640995259999,0,0,0,0,0\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test CSV: %v", err)
	}

	candles, stats, err := ReadCandlesCSV(path)
	if err != nil {
		t.Fatalf("read duplicate CSV: %v", err)
	}

	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, received %d", len(candles))
	}

	if stats.DuplicateRows != 1 {
		t.Fatalf("expected 1 duplicate, received %d", stats.DuplicateRows)
	}
}

func TestMalformedRowSkipped(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "malformed.csv")

	content := "" +
		"bad,row\n" +
		"1640995200000,100,105,99,104,10,1640995259999,0,0,0,0,0\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test CSV: %v", err)
	}

	candles, stats, err := ReadCandlesCSV(path)
	if err != nil {
		t.Fatalf("read malformed CSV: %v", err)
	}

	if len(candles) != 1 {
		t.Fatalf("expected 1 valid candle, received %d", len(candles))
	}

	if stats.MalformedRows != 1 {
		t.Fatalf("expected 1 malformed row, received %d", stats.MalformedRows)
	}
}
