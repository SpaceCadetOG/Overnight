package lighter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testManager(server *httptest.Server) *Manager {
	return &Manager{
		BaseURL:      server.URL,
		AccountIndex: 724535,
		HTTPClient:   server.Client(),
		authTokenFunc: func() (string, error) {
			return "test-token", nil
		},
	}
}

func TestNormalizeOrderState(t *testing.T) {
	tests := []struct {
		name  string
		order Order
		want  OrderState
	}{
		{"open", Order{Status: "open", InitialBaseAmount: "2", RemainingBaseAmount: "2", FilledBaseAmount: "0"}, OrderStateOpen},
		{"partial from amounts", Order{Status: "open", InitialBaseAmount: "2", RemainingBaseAmount: "1.25", FilledBaseAmount: "0.75"}, OrderStatePartial},
		{"filled status", Order{Status: "filled", InitialBaseAmount: "2", RemainingBaseAmount: "0"}, OrderStateFilled},
		{"filled from remaining", Order{Status: "closed", InitialBaseAmount: "2", RemainingBaseAmount: "0"}, OrderStateFilled},
		{"cancelled takes precedence", Order{Status: "cancelled", InitialBaseAmount: "2", RemainingBaseAmount: "1", FilledBaseAmount: "1"}, OrderStateCancelled},
		{"expired", Order{Status: "expired"}, OrderStateExpired},
		{"rejected", Order{Status: "rejected"}, OrderStateRejected},
		{"unknown", Order{Status: "new-status"}, OrderStateUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeOrderState(test.order); got != test.want {
				t.Fatalf("normalizeOrderState()=%s want=%s", got, test.want)
			}
		})
	}
}

func TestAverageFillPrice(t *testing.T) {
	order := Order{FilledBaseAmount: "0.25", FilledQuoteAmount: "500"}
	if got, want := averageFillPrice(order), "2000.000000000000000000"; got != want {
		t.Fatalf("averageFillPrice()=%q want=%q", got, want)
	}
}

func TestAccountOrdersInputLimits(t *testing.T) {
	m := &Manager{}
	if _, err := m.AccountOrders(t.Context(), nil); err == nil {
		t.Fatal("expected empty indexes error")
	}
	indexes := make([]int64, maxAccountOrderIndexes+1)
	for i := range indexes {
		indexes[i] = int64(i + 1)
	}
	if _, err := m.AccountOrders(t.Context(), indexes); err == nil {
		t.Fatal("expected too many indexes error")
	}
}

func TestInactiveOrdersInputLimits(t *testing.T) {
	m := &Manager{}
	for _, limit := range []int{0, maxInactiveOrdersLimit + 1} {
		if _, err := m.InactiveOrders(t.Context(), nil, "", limit); err == nil {
			t.Fatalf("expected limit=%d error", limit)
		}
	}
}

func TestAccountOrdersRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/accountOrders" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("account_index"); got != "724535" {
			t.Fatalf("account_index=%q", got)
		}
		if got := r.URL.Query().Get("client_order_indexes"); got != "10,20" {
			t.Fatalf("client_order_indexes=%q", got)
		}
		if got := r.Header.Get("Authorization"); got != "test-token" {
			t.Fatalf("authorization=%q", got)
		}
		fmt.Fprint(w, `{"code":200,"orders":[{"client_order_index":10,"status":"open"}]}`)
	}))
	defer server.Close()

	orders, err := testManager(server).AccountOrders(t.Context(), []int64{10, 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].ClientOrderIndex != 10 {
		t.Fatalf("orders=%+v", orders)
	}
}

func TestReconcileCancelledOrderFromInactiveHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accountOrders":
			fmt.Fprint(w, `{"code":200,"orders":[]}`)
		case "/api/v1/accountInactiveOrders":
			if got := r.URL.Query().Get("limit"); got != "100" {
				t.Fatalf("limit=%q", got)
			}
			fmt.Fprint(w, `{"code":200,"orders":[{"order_index":900,"client_order_index":44,"market_index":1,"initial_base_amount":"0.00020","remaining_base_amount":"0.00020","filled_base_amount":"0","filled_quote_amount":"0","status":"cancelled","updated_at":123}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := testManager(server).ReconcileOrder(t.Context(), 44)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != OrderStateCancelled || result.ExchangeOrderIndex != 900 {
		t.Fatalf("result=%+v", result)
	}
}

func TestReconcileFilledOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"orders":[{"order_index":901,"client_order_index":45,"market_index":0,"initial_base_amount":"0.0100","remaining_base_amount":"0","filled_base_amount":"0.0100","filled_quote_amount":"18.8225","status":"filled"}]}`)
	}))
	defer server.Close()

	result, err := testManager(server).ReconcileOrder(t.Context(), 45)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != OrderStateFilled {
		t.Fatalf("state=%s", result.State)
	}
	if result.AvgFillPrice != "1882.250000000000000000" {
		t.Fatalf("avg fill=%q", result.AvgFillPrice)
	}
}

func TestReconcileUnknownAfterHistoryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"orders":[]}`)
	}))
	defer server.Close()

	result, err := testManager(server).ReconcileOrder(t.Context(), 46)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != OrderStateUnknown || result.ClientOrderIndex != 46 {
		t.Fatalf("result=%+v", result)
	}
}
