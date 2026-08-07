package indicators

import (
	"math"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestCalculateFibonacci(t *testing.T) {
	session := models.Session{
		Low:  100,
		High: 200,
	}

	got, err := CalculateFibonacci(session)
	if err != nil {
		t.Fatalf("calculate Fibonacci: %v", err)
	}

	assertClose(t, "long 38.2", got.Long382, 138.2)
	assertClose(t, "long 50.0", got.Long500, 150.0)
	assertClose(t, "long 61.8", got.Long618, 161.8)

	assertClose(t, "short 38.2", got.Short382, 161.8)
	assertClose(t, "short 50.0", got.Short500, 150.0)
	assertClose(t, "short 61.8", got.Short618, 138.2)
}

func assertClose(t *testing.T, name string, got float64, want float64) {
	t.Helper()

	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("%s: got %.6f want %.6f", name, got, want)
	}
}
