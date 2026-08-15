package recordercert

import (
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
}
