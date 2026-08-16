package lighter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePositions(t *testing.T) {
	tests := []struct {
		name string
		raw  Position
		want PositionSide
	}{
		{"long", Position{Symbol: "BTC", Sign: 1, Size: "0.01"}, PositionSideLong},
		{"short", Position{Symbol: "ETH", Sign: -1, Size: "0.50"}, PositionSideShort},
		{"flat ignores stale sign", Position{Symbol: "SOL", Sign: 1, Size: "0"}, PositionSideFlat},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizePosition(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Side != test.want {
				t.Fatalf("side=%s want=%s", got.Side, test.want)
			}
		})
	}
}

func TestRejectPositionWithInvalidSign(t *testing.T) {
	_, err := normalizePosition(Position{Symbol: "BTC", Sign: 0, Size: "0.01"})
	if err == nil {
		t.Fatal("expected invalid sign error")
	}
}

func TestPositionSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/account" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("by") != "index" || query.Get("value") != "724535" || query.Get("active_only") != "true" {
			t.Fatalf("query=%v", query)
		}
		fmt.Fprint(w, `{"code":200,"accounts":[{"account_index":724535,"collateral":"1000","available_balance":"800","transaction_time":12345,"positions":[{"market_id":1,"symbol":"BTC","sign":1,"position":"0.01","avg_entry_price":"60000","position_value":"600","unrealized_pnl":"5","realized_pnl":"2","liquidation_price":"40000","margin_mode":0,"allocated_margin":"60"}]}],"next_cursor":""}`)
	}))
	defer server.Close()

	snapshot, err := testManager(server).PositionSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AccountIndex != 724535 || snapshot.TransactionTime != 12345 || len(snapshot.Positions) != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.Positions[0].Side != PositionSideLong {
		t.Fatalf("position=%+v", snapshot.Positions[0])
	}
}

func TestComparePositionsDetectsRecoveryDiscrepancies(t *testing.T) {
	snapshot := PositionSnapshot{Positions: []CanonicalPosition{
		{Symbol: "BTC", Side: PositionSideLong, Size: "0.005"},
		{Symbol: "SOL", Side: PositionSideShort, Size: "2"},
	}}
	expected := []PositionExpectation{
		{Symbol: "BTC", Side: PositionSideLong, Size: "0.010"},
		{Symbol: "ETH", Side: PositionSideLong, Size: "0.1"},
	}

	discrepancies, err := ComparePositions(snapshot, expected)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[PositionDiscrepancyKind]bool{}
	for _, discrepancy := range discrepancies {
		kinds[discrepancy.Kind] = true
	}
	for _, want := range []PositionDiscrepancyKind{PositionSizeMismatch, PositionUnexpected, PositionMissing} {
		if !kinds[want] {
			t.Fatalf("missing discrepancy %s: %+v", want, discrepancies)
		}
	}
}
