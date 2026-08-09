package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNtfyMessageContract(t *testing.T) {
	var title, priority, tags, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title, priority, tags = r.Header.Get("Title"), r.Header.Get("Priority"), r.Header.Get("Tags")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	c := New(server.URL, server.Client())
	if err := c.Send(context.Background(), "Runtime", "started", "high", "chart_with_upwards_trend"); err != nil {
		t.Fatal(err)
	}
	if title != "Runtime" || priority != "high" || tags == "" || body != "started" {
		t.Fatalf("unexpected message %q %q %q %q", title, priority, tags, body)
	}
}

func TestDisabledNtfyIsNoop(t *testing.T) {
	if err := (&Client{}).Send(context.Background(), "x", "y", "", ""); err != nil {
		t.Fatal(err)
	}
}
