package lighter

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecoveryStorePersistsIdempotencyMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	store, err := OpenRecoveryStore(path, 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := store.ReserveOrder("plan-2026-08-16-BTC-entry", 1234, "btc", 1)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.Symbol != "BTC" || mapping.SubmissionState != SubmissionReserved {
		t.Fatalf("mapping=%+v", mapping)
	}
	if err := store.MarkPrepared(mapping.IntentKey, 77, 20, 600000, 0.00020, 60000.0); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSubmitted(mapping.IntentKey, "0xabc"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenRecoveryStore(path, 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	saved := reopened.Snapshot().Orders[mapping.IntentKey]
	if saved.ClientOrderIndex != 1234 || saved.TxHash != "0xabc" || saved.SubmissionState != SubmissionSubmitted ||
		saved.Nonce != 77 || saved.EncodedBaseAmount != 20 || saved.EncodedPrice != 600000 ||
		saved.RequestedQuantity != 0.00020 || saved.RequestedPrice != 60000.0 {
		t.Fatalf("saved=%+v", saved)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions=%o", info.Mode().Perm())
	}
}

func TestRecoveryStoreRejectsDuplicateIntent(t *testing.T) {
	store, err := OpenRecoveryStore(filepath.Join(t.TempDir(), "state.json"), 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.ReserveOrder("same-intent", 100, "BTC", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ReserveOrder("same-intent", 999, "BTC", 1)
	if !errors.Is(err, ErrDuplicateOrderIntent) {
		t.Fatalf("error=%v", err)
	}
	if second.ClientOrderIndex != first.ClientOrderIndex {
		t.Fatalf("duplicate changed client ID: first=%d second=%d", first.ClientOrderIndex, second.ClientOrderIndex)
	}
}

func TestRecoveryStorePersistsExchangeOrderIndexFromStream(t *testing.T) {
	store, err := OpenRecoveryStore(filepath.Join(t.TempDir(), "state.json"), 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveOrder("btc-entry", 100, "BTC", 1); err != nil {
		t.Fatal(err)
	}
	order := &ReconciledOrder{ClientOrderIndex: 100, ExchangeOrderIndex: 9001, MarketIndex: 1, State: OrderStateOpen}
	if err := store.MarkReconciledByClientOrderIndex(order); err != nil {
		t.Fatal(err)
	}
	saved := store.Snapshot().Orders["btc-entry"]
	if saved.ExchangeOrderIndex != 9001 || saved.LastOrder == nil || saved.LastOrder.State != OrderStateOpen {
		t.Fatalf("saved=%+v", saved)
	}
}

func TestRecoveryStoreRejectsWrongIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenRecoveryStore(path, 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveOrder("intent", 1, "BTC", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRecoveryStore(path, 999, 8); err == nil {
		t.Fatal("expected account identity error")
	}
}

func TestNonceCoordinatorAllocatesUniqueNonces(t *testing.T) {
	coordinator, err := NewNonceCoordinator(50)
	if err != nil {
		t.Fatal(err)
	}
	const count = 100
	results := make(chan int64, count)
	var wait sync.WaitGroup
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- coordinator.Take()
		}()
	}
	wait.Wait()
	close(results)
	seen := make(map[int64]bool, count)
	for nonce := range results {
		if seen[nonce] {
			t.Fatalf("duplicate nonce %d", nonce)
		}
		seen[nonce] = true
	}
	if len(seen) != count || coordinator.Peek() != 150 {
		t.Fatalf("count=%d next=%d", len(seen), coordinator.Peek())
	}
}

func TestRecoverReconstructsExchangeState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nextNonce":
			fmt.Fprint(w, `{"code":200,"nonce":77}`)
		case "/api/v1/accountActiveOrders":
			fmt.Fprint(w, `{"code":200,"orders":[{"order_index":10,"client_order_index":100,"market_index":1,"status":"open","initial_base_amount":"1","remaining_base_amount":"1","filled_base_amount":"0"},{"order_index":11,"client_order_index":999,"market_index":0,"status":"open","initial_base_amount":"1","remaining_base_amount":"1","filled_base_amount":"0"}]}`)
		case "/api/v1/accountOrders":
			if r.URL.Query().Get("client_order_indexes") != "100" {
				t.Fatalf("client order query=%q", r.URL.Query().Get("client_order_indexes"))
			}
			fmt.Fprint(w, `{"code":200,"orders":[{"order_index":10,"client_order_index":100,"market_index":1,"status":"open","initial_base_amount":"1","remaining_base_amount":"1","filled_base_amount":"0"}]}`)
		case "/api/v1/account":
			fmt.Fprint(w, `{"code":200,"accounts":[{"account_index":724535,"collateral":"1000","available_balance":"900","transaction_time":123,"positions":[{"market_id":1,"symbol":"BTC","sign":1,"position":"0.01"}]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store, err := OpenRecoveryStore(filepath.Join(t.TempDir(), "state.json"), 724535, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveOrder("btc-entry", 100, "BTC", 1); err != nil {
		t.Fatal(err)
	}
	store.state.PositionSnapshot = &PositionSnapshot{Positions: []CanonicalPosition{
		{Symbol: "BTC", Side: PositionSideLong, Size: "0.02"},
	}}
	if err := store.persistLocked(); err != nil {
		t.Fatal(err)
	}
	manager := testManager(server)
	manager.APIKeyIndex = 8
	report, nonces, err := store.Recover(t.Context(), manager)
	if err != nil {
		t.Fatal(err)
	}
	if report.ExchangeNextNonce != 77 || nonces.Peek() != 77 {
		t.Fatalf("report nonce=%d coordinator=%d", report.ExchangeNextNonce, nonces.Peek())
	}
	if len(report.TrackedOrders) != 1 || report.TrackedOrders[0].State != OrderStateOpen {
		t.Fatalf("tracked=%+v", report.TrackedOrders)
	}
	if len(report.UntrackedActiveOrders) != 1 || report.UntrackedActiveOrders[0].ClientOrderIndex != 999 {
		t.Fatalf("untracked=%+v", report.UntrackedActiveOrders)
	}
	if report.Positions == nil || len(report.Positions.Positions) != 1 {
		t.Fatalf("positions=%+v", report.Positions)
	}
	if len(report.PositionDiscrepancies) != 1 || report.PositionDiscrepancies[0].Kind != PositionSizeMismatch {
		t.Fatalf("position discrepancies=%+v", report.PositionDiscrepancies)
	}
	saved := store.Snapshot()
	if saved.LastObservedNonce != 77 || saved.PositionSnapshot == nil || saved.Orders["btc-entry"].LastOrder == nil {
		t.Fatalf("saved=%+v", saved)
	}
}
