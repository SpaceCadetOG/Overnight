package lighterexec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	lighteradapter "github.com/ogtrading/lighter-adapter"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestReadReconciliationSnapshotHistoricalOrdersUseLimitAndSkipInactiveMarkets(t *testing.T) {
	t.Helper()

	var historicalCalls int
	client := &Client{
		cfg: Config{
			BaseURL:      "https://lighter.test",
			AccountIndex: 724535,
		},
		http: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{}`
				status := http.StatusOK
				headers := http.Header{"Content-Type": []string{"application/json"}}

				switch r.URL.Path {
				case "/api/v1/account":
					body = marshalJSON(t, map[string]any{
						"accounts": []map[string]any{
							{
								"account_index": 724535,
								"positions":     []any{},
							},
						},
					})
				case "/api/v1/accountActiveOrders":
					marketID := r.URL.Query().Get("market_id")
					if marketID == "99" {
						t.Fatalf("inactive market should not be queried for active orders")
					}
					if r.URL.Query().Get("auth") != "test-auth" {
						t.Fatalf("expected auth token on active orders query")
					}
					body = marshalJSON(t, map[string]any{"orders": []map[string]any{}})
				case "/api/v1/trades":
					body = marshalJSON(t, map[string]any{"trades": []map[string]any{}})
				case "/api/v1/accountInactiveOrders":
					historicalCalls++
					marketID := r.URL.Query().Get("market_id")
					if marketID == "99" {
						t.Fatalf("inactive market should not be queried for historical orders")
					}
					if got := r.URL.Query().Get("limit"); got != "100" {
						t.Fatalf("expected historical orders limit=100, got %q", got)
					}
					if r.URL.Query().Get("auth") != "test-auth" {
						t.Fatalf("expected auth token on historical orders query")
					}
					body = marshalJSON(t, map[string]any{"orders": []map[string]any{}})
				case "/api/v1/positionFunding":
					body = marshalJSON(t, map[string]any{"position_fundings": []map[string]any{}, "fundings": []map[string]any{}})
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}

				return &http.Response{
					StatusCode: status,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
		authTokenFn: func() (string, error) {
			return "test-auth", nil
		},
	}
	client.readonly = lighteradapter.New(lighteradapter.Config{
		BaseURL:    client.cfg.BaseURL,
		HTTPClient: client.http,
		AuthTokenProvider: lighteradapter.AuthTokenFunc(func(_ time.Time) (string, error) {
			return "test-auth", nil
		}),
	})

	snapshot, err := client.ReadReconciliationSnapshot(context.Background(), []Market{
		{Symbol: "ETH", MarketID: 0, Status: "active"},
		{Symbol: "INACTIVE", MarketID: 99, Status: "inactive"},
	})
	if err != nil {
		t.Fatalf("expected snapshot to succeed, got error: %v", err)
	}
	if historicalCalls != 1 {
		t.Fatalf("expected one historical orders call, got %d", historicalCalls)
	}
	if snapshot.Endpoints["historical_orders"].State != "fresh" {
		t.Fatalf("expected historical_orders endpoint to be fresh, got %q", snapshot.Endpoints["historical_orders"].State)
	}
	if snapshot.Endpoints["historical_orders"].RetrievedAt.IsZero() {
		t.Fatalf("expected historical_orders retrieval timestamp to be set")
	}
}

func TestReadReconciliationSnapshotUsesCachedHistoricalOrdersWhenEndpointTurnsStale(t *testing.T) {
	t.Helper()

	callCount := 0
	client := &Client{
		cfg: Config{
			BaseURL:      "https://lighter.test",
			AccountIndex: 724535,
		},
		http: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{}`
				status := http.StatusOK
				headers := http.Header{"Content-Type": []string{"application/json"}}

				switch r.URL.Path {
				case "/api/v1/account":
					body = marshalJSON(t, map[string]any{
						"accounts": []map[string]any{
							{
								"account_index": 724535,
								"positions":     []any{},
							},
						},
					})
				case "/api/v1/accountActiveOrders":
					body = marshalJSON(t, map[string]any{"orders": []map[string]any{}})
				case "/api/v1/trades":
					body = marshalJSON(t, map[string]any{"trades": []map[string]any{}})
				case "/api/v1/accountInactiveOrders":
					callCount++
					if callCount == 1 {
						body = marshalJSON(t, map[string]any{"orders": []map[string]any{{"order_id": "1"}}})
						break
					}
					status = http.StatusBadGateway
					body = `{"message":"temporary error"}`
				case "/api/v1/positionFunding":
					body = marshalJSON(t, map[string]any{"position_fundings": []map[string]any{}, "fundings": []map[string]any{}})
				default:
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}

				return &http.Response{
					StatusCode: status,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
		authTokenFn: func() (string, error) {
			return "test-auth", nil
		},
	}
	client.readonly = lighteradapter.New(lighteradapter.Config{
		BaseURL:    client.cfg.BaseURL,
		HTTPClient: client.http,
		AuthTokenProvider: lighteradapter.AuthTokenFunc(func(_ time.Time) (string, error) {
			return "test-auth", nil
		}),
	})

	first, err := client.ReadReconciliationSnapshot(context.Background(), []Market{{Symbol: "ETH", MarketID: 0, Status: "active"}})
	if err != nil {
		t.Fatalf("expected first snapshot to succeed, got %v", err)
	}
	if len(first.HistoricalOrders) != 1 {
		t.Fatalf("expected first snapshot to cache one historical order, got %d", len(first.HistoricalOrders))
	}

	client.historicalAt = time.Now().UTC().Add(-time.Minute)
	second, err := client.ReadReconciliationSnapshot(context.Background(), []Market{{Symbol: "ETH", MarketID: 0, Status: "active"}})
	if err == nil {
		t.Fatalf("expected stale historical endpoint to surface an error")
	}
	if len(second.HistoricalOrders) != 1 {
		t.Fatalf("expected cached historical orders to remain available, got %d", len(second.HistoricalOrders))
	}
	if second.Endpoints["historical_orders"].State != "stale" {
		t.Fatalf("expected stale historical_orders state, got %q", second.Endpoints["historical_orders"].State)
	}
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(data)
}
