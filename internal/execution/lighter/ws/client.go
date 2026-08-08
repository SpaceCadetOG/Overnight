package ws

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn *websocket.Conn
}

func Connect(url string) (*Client, error) {

	conn, _, err := websocket.DefaultDialer.Dial(
		url,
		nil,
	)

	if err != nil {
		return nil, err
	}

	return &Client{
		conn: conn,
	}, nil
}

func (c *Client) Send(v any) error {

	return c.conn.WriteJSON(v)
}

func (c *Client) Read() ([]byte, error) {

	c.conn.SetReadDeadline(
		time.Now().Add(60 * time.Second),
	)

	_, msg, err := c.conn.ReadMessage()

	return msg, err
}

func (c *Client) Close() {
	c.conn.Close()
}

func Decode(data []byte) map[string]any {

	var out map[string]any

	json.Unmarshal(data, &out)

	return out
}
