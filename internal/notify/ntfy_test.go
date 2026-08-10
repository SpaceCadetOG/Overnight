package notify

import (
	"context"
	"encoding/json"
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

func TestTelegramMessageContract(t *testing.T) {
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type=%q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	c := NewTelegram(server.URL, "12345", server.Client())
	if err := c.Send(context.Background(), "Overnight EOD PASS", "Result: +1.00R", "default", "bar_chart"); err != nil {
		t.Fatal(err)
	}
	if payload["chat_id"] != "12345" || payload["text"] != "Overnight EOD PASS\nResult: +1.00R" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestTelegramCommandNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"/scanner":                  "/scanner",
		" /STATUS@OvernightBot now": "/status",
		"":                          "",
	} {
		if got := telegramCommand(input); got != want {
			t.Fatalf("command %q=%q want %q", input, got, want)
		}
	}
}
