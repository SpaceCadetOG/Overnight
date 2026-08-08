package execution

import "testing"

func TestClientOrderIndexIsStableAndDistinct(t *testing.T) {
	a, err := ClientOrderIndex("20260810-BTC")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ClientOrderIndex("20260810-BTC")
	c, _ := ClientOrderIndex("20260810-ETH")
	if a != b || a == c || a <= 0 || a >= 1<<48 {
		t.Fatalf("unexpected indexes %d %d %d", a, b, c)
	}
}
