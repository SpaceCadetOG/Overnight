package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}
}

type receiptMemory struct{ rows []map[string]any }

func (m *receiptMemory) Append(_ string, value any) error {
	body, _ := json.Marshal(value)
	var row map[string]any
	_ = json.Unmarshal(body, &row)
	m.rows = append(m.rows, row)
	return nil
}

func TestNtfyMessageContract(t *testing.T) {
	var title, priority, tags, body string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		title, priority, tags = r.Header.Get("Title"), r.Header.Get("Priority"), r.Header.Get("Tags")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		return response(http.StatusOK), nil
	})}
	receipts := &receiptMemory{}
	c := New("https://notify.invalid/topic", client).WithReceiptStore(receipts)
	if err := c.Send(context.Background(), "Runtime", "started", "high", "chart_with_upwards_trend"); err != nil {
		t.Fatal(err)
	}
	if title != "Runtime" || priority != "high" || tags == "" || body != "started" {
		t.Fatalf("unexpected message %q %q %q %q", title, priority, tags, body)
	}
	if len(receipts.rows) != 2 || receipts.rows[0]["state"] != "ATTEMPTED" || receipts.rows[1]["state"] != "DELIVERED" {
		t.Fatalf("delivery receipts=%#v", receipts.rows)
	}
}

func TestDisabledNtfyIsNoop(t *testing.T) {
	if err := (&Client{}).Send(context.Background(), "x", "y", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestTelegramMessageContract(t *testing.T) {
	var payload map[string]string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type=%q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return response(http.StatusOK), nil
	})}
	c := NewTelegram("https://telegram.invalid/sendMessage", "12345", client)
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
