package recordercert

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineReplayCertificate(t *testing.T) {
	root := t.TempDir()
	symbols := []string{"BTC", "ETH", "SOL", "HYPE", "LIT", "XAU", "XAG", "LINK", "AAVE", "UNI", "ZEC", "BNB"}
	files := map[string]string{
		"orderbook_events.jsonl":               "{\"received_at\":\"2026-08-10T00:00:01Z\",\"event\":{\"type\":\"subscribed/order_book\",\"order_book\":{\"begin_nonce\":0,\"nonce\":10,\"bids\":[{\"price\":\"99\",\"size\":\"2\"}],\"asks\":[{\"price\":\"101\",\"size\":\"3\"}]}}}\n{\"received_at\":\"2026-08-10T23:59:59Z\",\"event\":{\"type\":\"update/order_book\",\"order_book\":{\"begin_nonce\":10,\"nonce\":11,\"bids\":[{\"price\":\"99\",\"size\":\"4\"}],\"asks\":[]}}}\n",
		"reconstructed_book_checkpoints.jsonl": "{\"nonce\":10,\"best_bid\":99,\"best_ask\":101}\n{\"nonce\":11,\"best_bid\":99,\"best_ask\":101}\n", "ticker_events.jsonl": "{}\n", "trade_flow.jsonl": "{}\n"}
	for _, symbol := range symbols {
		dir := filepath.Join(root, "asset="+symbol)
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatal(err)
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0640); err != nil {
				t.Fatal(err)
			}
		}
	}
	cert, err := Certify(root, symbols)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.Pass || cert.Ready != 12 || cert.Expected != 12 || !cert.SnapshotComparisons {
		t.Fatalf("cert=%+v", cert)
	}
	if cert.Classification != "CERTIFIED" || cert.TotalWindows != 12 || cert.CertifiedWindows != 12 {
		t.Fatalf("window certification=%+v", cert)
	}
}

func TestDayBoundaryDeltaPrefixIsExcludedUntilSnapshot(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "asset=BTC")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{
		{"received_at": "2026-09-09T05:00:00Z", "connection_id": "old", "event": map[string]any{"type": "update/order_book", "order_book": map[string]any{"begin_nonce": 8, "nonce": 9, "bids": []any{}, "asks": []any{}}}},
		{"received_at": "2026-09-09T05:01:00Z", "connection_id": "new", "event": map[string]any{"type": "subscribed/order_book", "order_book": map[string]any{"begin_nonce": 0, "nonce": 10, "bids": []any{map[string]any{"price": "99", "size": "2"}}, "asks": []any{map[string]any{"price": "101", "size": "3"}}}}},
		{"received_at": "2026-09-10T04:59:59Z", "connection_id": "new", "event": map[string]any{"type": "update/order_book", "order_book": map[string]any{"begin_nonce": 10, "nonce": 11, "bids": []any{map[string]any{"price": "99", "size": "4"}}, "asks": []any{}}}},
	}
	file, err := os.Create(filepath.Join(dir, "orderbook_events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"reconstructed_book_checkpoints.jsonl": "{\"nonce\":10,\"best_bid\":99,\"best_ask\":101}\n{\"nonce\":11,\"best_bid\":99,\"best_ask\":101}\n",
		"ticker_events.jsonl":                  "{}\n", "trade_flow.jsonl": "{}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0640); err != nil {
			t.Fatal(err)
		}
	}
	cert, err := Certify(root, []string{"BTC"})
	if err != nil {
		t.Fatal(err)
	}
	asset := cert.Assets[0]
	if asset.PreSnapshotEvents != 1 || asset.CertifiedWindows != 1 || len(asset.Windows) != 1 {
		t.Fatalf("asset=%+v", asset)
	}
	if cert.Classification != "CERTIFIED_WITH_EXCLUDED_INTERVALS" {
		t.Fatalf("classification=%s", cert.Classification)
	}
}
