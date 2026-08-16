package lighter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestEncodeOrderOptions(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name    string
		request PlaceOrderRequest
		wantErr bool
	}{
		{"limit post-only", PlaceOrderRequest{Type: ExecutionOrderLimit, TimeInForce: TimeInForcePostOnly, ExpiresAt: future}, false},
		{"market IOC", PlaceOrderRequest{Type: ExecutionOrderMarket, TimeInForce: TimeInForceIOC}, false},
		{"market cannot rest", PlaceOrderRequest{Type: ExecutionOrderMarket, TimeInForce: TimeInForceGoodTill, ExpiresAt: future}, true},
		{"stop IOC with expiry", PlaceOrderRequest{Type: ExecutionOrderStopLoss, TimeInForce: TimeInForceIOC, ExpiresAt: future}, false},
		{"trigger requires expiry", PlaceOrderRequest{Type: ExecutionOrderTakeProfit, TimeInForce: TimeInForceIOC}, true},
		{"stop limit can rest", PlaceOrderRequest{Type: ExecutionOrderStopLossLimit, TimeInForce: TimeInForceGoodTill, ExpiresAt: future}, false},
		{"take profit limit can post", PlaceOrderRequest{Type: ExecutionOrderTakeProfitLimit, TimeInForce: TimeInForcePostOnly, ExpiresAt: future}, false},
		{"trigger limit cannot IOC", PlaceOrderRequest{Type: ExecutionOrderStopLossLimit, TimeInForce: TimeInForceIOC, ExpiresAt: future}, true},
		{"resting requires expiry", PlaceOrderRequest{Type: ExecutionOrderLimit, TimeInForce: TimeInForcePostOnly}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := encodeOrderOptions(test.request)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestExecutionErrorClassification(t *testing.T) {
	tests := []struct {
		err       error
		kind      ErrorKind
		retryable bool
	}{
		{ErrDuplicateOrderIntent, ErrorConflict, false},
		{errors.New("HTTP 429"), ErrorRateLimit, true},
		{errors.New("HTTP 401"), ErrorAuth, false},
		{errors.New("quantity is below minimum"), ErrorValidation, false},
	}
	for _, test := range tests {
		got := classifyExecutionError("test", test.err)
		if got.Kind != test.kind || got.Retryable != test.retryable {
			t.Fatalf("error=%v got=%+v", test.err, got)
		}
	}
}

func TestRetryReadRetriesOnlyRetryableFailure(t *testing.T) {
	attempts := 0
	value, err := retryRead(context.Background(), 3, "read", func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errors.New("HTTP 503")
		}
		return 42, nil
	})
	if err != nil || value != 42 || attempts != 3 {
		t.Fatalf("value=%d attempts=%d error=%v", value, attempts, err)
	}
}

func TestPlaceOrderRejectsPersistedDuplicateBeforeSigning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/orderBookDetails" {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"code":200,"order_book_details":[{"symbol":"BTC","market_id":37,"market_type":"perp","status":"active","min_base_amount":"0.001","min_quote_amount":"10","supported_size_decimals":3,"supported_price_decimals":2}]}`)
	}))
	defer server.Close()

	store, err := OpenRecoveryStore(filepath.Join(t.TempDir(), "state.json"), 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveOrder("daily-plan:BTC:entry", 100, "BTC", 37); err != nil {
		t.Fatal(err)
	}
	manager := testManager(server)
	manager.APIKeyIndex = 8
	nonces, _ := NewNonceCoordinator(1)
	engine := &ExecutionEngine{manager: manager, store: store, nonces: nonces, nextClientID: 101, readAttempts: 1}

	_, err = engine.PlaceOrder(t.Context(), PlaceOrderRequest{
		IntentKey: "daily-plan:BTC:entry", Symbol: "BTC", Side: SideBuy,
		Quantity: 0.01, Price: 60000, Type: ExecutionOrderLimit,
		TimeInForce: TimeInForcePostOnly, ExpiresAt: time.Now().Add(time.Hour),
	})
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) || executionErr.Kind != ErrorConflict {
		t.Fatalf("error=%v", err)
	}
	if nonces.Peek() != 1 {
		t.Fatalf("nonce consumed for duplicate intent: %d", nonces.Peek())
	}
}

func TestHTTPStatusRetryable(t *testing.T) {
	if !HTTPStatusRetryable(429) || !HTTPStatusRetryable(503) || HTTPStatusRetryable(400) {
		t.Fatal("unexpected HTTP retry classification")
	}
}
