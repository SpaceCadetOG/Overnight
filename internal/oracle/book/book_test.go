package book

import (
	"errors"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

func TestSnapshotDeltaDeleteDepthAndRestore(t *testing.T) {
	s := New("BTC")
	if err := s.ApplySnapshot(model.Book{Nonce: 10, Bids: []model.Level{{Price: "99.00", Size: "2"}, {Price: "98", Size: "3"}}, Asks: []model.Level{{Price: "101.00", Size: "4"}, {Price: "102", Size: "5"}}}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyDelta(model.Book{BeginNonce: 10, Nonce: 12, Bids: []model.Level{{Price: "99.00", Size: "0"}, {Price: "100", Size: "6"}}, Asks: []model.Level{{Price: "101.00", Size: "7"}}}, 2); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.Snapshot(1, time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC), "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.Bids) != 1 || checkpoint.Bids[0].Price != "100" || checkpoint.Asks[0].Size != "7" || checkpoint.Checksum == "" {
		t.Fatalf("checkpoint=%+v", checkpoint)
	}
	restored, err := Restore(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	again, err := restored.Snapshot(1, checkpoint.At, checkpoint.ConnectionID)
	if err != nil || again.Checksum != checkpoint.Checksum {
		t.Fatalf("restore differs: %+v err=%v", again, err)
	}
}

func TestGapInvalidatesUntilFreshSnapshot(t *testing.T) {
	s := New("ETH")
	if err := s.ApplySnapshot(model.Book{Nonce: 10}, 4); err != nil {
		t.Fatal(err)
	}
	err := s.ApplyDelta(model.Book{BeginNonce: 11, Nonce: 12}, 5)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("gap did not invalidate: %v", err)
	}
	if _, err := s.Snapshot(250, time.Now(), "conn"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid book was published: %v", err)
	}
	if err := s.ApplyDelta(model.Book{BeginNonce: 10, Nonce: 11}, 5); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("delta resumed without snapshot: %v", err)
	}
	if err := s.ApplySnapshot(model.Book{Nonce: 20, Bids: []model.Level{{Price: "100", Size: "1"}}, Asks: []model.Level{{Price: "101", Size: "1"}}}, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(250, time.Now(), "new-conn"); err != nil {
		t.Fatal(err)
	}
}

func TestCrossedAndInvalidLevelsCannotBePublished(t *testing.T) {
	for _, value := range []model.Book{
		{Nonce: 1, Bids: []model.Level{{Price: "101", Size: "1"}}, Asks: []model.Level{{Price: "100", Size: "1"}}},
		{Nonce: 1, Bids: []model.Level{{Price: "99", Size: "-1"}}},
		{Nonce: 1, Asks: []model.Level{{Price: "bad", Size: "1"}}},
	} {
		s := New("ZEC")
		if err := s.ApplySnapshot(value, 1); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("invalid snapshot accepted: %+v err=%v", value, err)
		}
	}
}

func TestTamperedCheckpointIsRejected(t *testing.T) {
	s := New("BTC")
	if err := s.ApplySnapshot(model.Book{Nonce: 1, Bids: []model.Level{{Price: "99", Size: "1"}}}, 1); err != nil {
		t.Fatal(err)
	}
	cp, _ := s.Snapshot(250, time.Now(), "conn")
	cp.Bids[0].Size = "999"
	if _, err := Restore(cp); err == nil {
		t.Fatal("tampered checkpoint restored")
	}
}

func TestTopLevelsSelectsBestPrices(t *testing.T) {
	side := map[string]string{"99.5": "1", "101": "2", "98": "3", "100.25": "4", "102": "5"}
	bids := topLevels(side, true, 3)
	asks := topLevels(side, false, 3)
	for index, expected := range []string{"102", "101", "100.25"} {
		if bids[index].Price != expected {
			t.Fatalf("bid[%d]=%s want=%s", index, bids[index].Price, expected)
		}
	}
	for index, expected := range []string{"98", "99.5", "100.25"} {
		if asks[index].Price != expected {
			t.Fatalf("ask[%d]=%s want=%s", index, asks[index].Price, expected)
		}
	}
}
