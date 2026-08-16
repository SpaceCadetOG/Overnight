package lighteradapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func (c *Client) CheckPrivateWebSocket(ctx context.Context, wsURL string, accountIndex int64) (PrivateWSCheckResult, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(wsURL) == "" {
		wsURL = c.wsURL
	}
	token, err := c.authToken()
	if err != nil {
		result := PrivateWSCheckResult{
			Channel:       "account_all_positions/" + strconv.FormatInt(accountIndex, 10),
			Freshness:     FreshnessFailed,
			RetrievedAt:   now,
			Error:         err.Error(),
			Authoritative: false,
		}
		return result, err
	}

	headers := http.Header{
		"Origin":     []string{"https://lighter.xyz"},
		"User-Agent": []string{"lighter-adapter/1.0"},
	}

	var conn *websocket.Conn
	for attempt := 1; attempt <= 3; attempt++ {
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, wsURL, headers)
		if err == nil {
			break
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				result := PrivateWSCheckResult{
					Channel:       "account_all_positions/" + strconv.FormatInt(accountIndex, 10),
					Freshness:     FreshnessFailed,
					RetrievedAt:   now,
					Error:         ctx.Err().Error(),
					Authoritative: false,
				}
				return result, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	channel := "account_all_positions/" + strconv.FormatInt(accountIndex, 10)
	result := PrivateWSCheckResult{
		Channel:       channel,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = fmt.Errorf("connect private WebSocket after 3 attempts: %w", err).Error()
		return result, fmt.Errorf("%s", result.Error)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{"type": "subscribe", "channel": channel, "auth": token}); err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = fmt.Errorf("subscribe private WebSocket: %w", err).Error()
		return result, fmt.Errorf("%s", result.Error)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
	_, message, err := conn.ReadMessage()
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = fmt.Errorf("read private WebSocket: %w", err).Error()
		return result, fmt.Errorf("%s", result.Error)
	}
	if errorValue := strings.TrimSpace(extractError(message)); errorValue != "" && errorValue != "<nil>" {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = "private WebSocket rejected subscription: " + errorValue
		return result, fmt.Errorf("%s", result.Error)
	}
	return result, nil
}

func extractError(message []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(message, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(payload["error"]))
}
