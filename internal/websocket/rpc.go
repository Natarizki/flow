package websocket

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

const defaultRPCTimeout = 8 * time.Second

// RPCTracker melacak request yang nunggu balesan, dikeyed by message ID.
// Setiap Client punya satu, dipakai buat DHT request/response yang
// sifatnya sinkron dari sudut pandang caller walau transportnya async.
type RPCTracker struct {
	pending map[string]chan *Message
	mu      sync.Mutex
}

func NewRPCTracker() *RPCTracker {
	return &RPCTracker{pending: make(map[string]chan *Message)}
}

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Call kirim message lewat client dan blok sampe balesan dengan ID yang
// sama masuk, atau timeout.
func (t *RPCTracker) Call(c *Client, msg *Message, timeout time.Duration) (*Message, error) {
	if msg.ID == "" {
		msg.ID = generateRequestID()
	}

	ch := make(chan *Message, 1)
	t.mu.Lock()
	t.pending[msg.ID] = ch
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.pending, msg.ID)
		t.mu.Unlock()
	}()

	c.SendMessage(msg)

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("rpc timeout waiting for response to %s", msg.ID)
	}
}

// Resolve dipanggil dari message dispatcher tiap kali ada message masuk
// yang punya ID — kalau ada pending call yang nunggu ID itu, forward.
// Return true kalau message ini beneran ditangani sebagai RPC response
// (artinya dispatcher gak perlu proses lebih jauh sebagai command baru).
func (t *RPCTracker) Resolve(msg *Message) bool {
	if msg.ID == "" {
		return false
	}
	t.mu.Lock()
	ch, ok := t.pending[msg.ID]
	t.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- msg:
	default:
		utils.LogWarn("rpc: response for %s arrived but channel full/closed", msg.ID)
	}
	return true
}

func DefaultTimeout() time.Duration {
	return defaultRPCTimeout
}
