package websocket

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Natarizki/flow/pkg/utils"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 32 * 1024 * 1024 // 32MB per chunk message
)

type Client struct {
	PeerID    string
	PublicKey string
	Conn      *websocket.Conn
	Send      chan *Message
	Hub       *Hub
}

func NewClient(peerID string, conn *websocket.Conn, hub *Hub) *Client {
	return &Client{
		PeerID: peerID,
		Conn:   conn,
		Send:   make(chan *Message, 64),
		Hub:    hub,
	}
}

func (c *Client) ReadPump(handler *Handler) {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				utils.LogError("read error from %s: %v", c.PeerID, err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			utils.LogWarn("invalid message from %s: %v", c.PeerID, err)
			continue
		}
		msg.PeerID = c.PeerID

		handler.Dispatch(c, &msg)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				utils.LogError("marshal error: %v", err)
				continue
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) SendMessage(msg *Message) {
	select {
	case c.Send <- msg:
	default:
		utils.LogWarn("send buffer full for peer %s, dropping message", c.PeerID)
	}
}
