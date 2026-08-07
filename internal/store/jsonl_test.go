package store

import "testing"

func TestJSONLRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewJSONL(root)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID int `json:"id"`
	}
	if err := store.Append("events", row{ID: 7}); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAll[row](root, "events")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != 7 {
		t.Fatalf("rows=%+v", rows)
	}
}
