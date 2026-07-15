package websocket

import (
	"sync"

	"github.com/Natarizki/flow/pkg/utils"
)

type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Message
	rpc        *RPCTracker
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		rpc:        NewRPCTracker(),
	}
}

// RPC balikin RPCTracker milik Hub — dipakai buat semua request/response
// correlation, baik DHT (find_node, find_value) maupun chunk pull
// (chunk_request -> chunk_response), jadi cuma ada satu tracker per Hub,
// bukan satu per fitur.
func (h *Hub) RPC() *RPCTracker {
	return h.rpc
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.PeerID] = client
			h.mu.Unlock()
			utils.LogInfo("peer connected: %s (total: %d)", client.PeerID, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.PeerID]; ok {
				delete(h.clients, client.PeerID)
				close(client.Send)
			}
			h.mu.Unlock()
			utils.LogInfo("peer disconnected: %s (total: %d)", client.PeerID, len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- msg:
				default:
					close(client.Send)
					delete(h.clients, client.PeerID)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Register(c *Client)     { h.register <- c }
func (h *Hub) Unregister(c *Client)   { h.unregister <- c }
func (h *Hub) Broadcast(msg *Message) { h.broadcast <- msg }

func (h *Hub) Rekey(oldID, newID string) {
	if oldID == newID || newID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	c, ok := h.clients[oldID]
	if !ok {
		return
	}
	delete(h.clients, oldID)
	c.PeerID = newID
	h.clients[newID] = c
	utils.LogInfo("rekeyed peer connection: %s -> %s", oldID, newID)
}

func (h *Hub) GetClient(peerID string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[peerID]
	return c, ok
}

func (h *Hub) PeerCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) ListPeers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	peers := make([]string, 0, len(h.clients))
	for id := range h.clients {
		peers = append(peers, id)
	}
	return peers
}
