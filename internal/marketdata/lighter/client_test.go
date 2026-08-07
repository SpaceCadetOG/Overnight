package lighter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCandlesParsesLighterSchema(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":200,"c":[{"t":1000,"o":1,"h":2,"l":0.5,"c":1.5,"v":8}]}`))}, nil
	})}
	rows, err := New("https://example.test", httpClient).Candles(context.Background(), 1, "5m", time.UnixMilli(0), time.UnixMilli(300000))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Close != 1.5 || rows[0].Volume != 8 {
		t.Fatalf("rows=%+v", rows)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
