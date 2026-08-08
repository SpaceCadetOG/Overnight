package execution

import (
	"testing"
	"time"
)

func TestOrderWindowConvertsChicagoToUTCWithDST(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		date string
		want string
	}{
		{"2026-08-10", "2026-08-10T10:00:00Z"},
		{"2026-12-10", "2026-12-10T11:00:00Z"},
	}
	for _, test := range tests {
		date, err := time.Parse("2006-01-02", test.date)
		if err != nil {
			t.Fatal(err)
		}
		start, end := OrderWindow(date, chicago)
		if start.Format(time.RFC3339) != test.want {
			t.Fatalf("%s start=%s want=%s", test.date, start.Format(time.RFC3339), test.want)
		}
		if end.Sub(start) != 5*time.Minute {
			t.Fatalf("window duration=%s", end.Sub(start))
		}
	}
}
