package websocket

import (
	"encoding/json"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

type Handler struct {
	hub *Hub

	onChunkRequest  func(c *Client, msg *Message, p ChunkRequestPayload)
	onChunkResponse func(c *Client, p ChunkResponsePayload)
	onChunkHave     func(c *Client, p ChunkHavePayload)
	onPeerAnnounce  func(c *Client, p PeerAnnouncePayload)
	onHandshake     func(c *Client, p HandshakePayload)
	onDHTMessage    func(c *Client, msg *Message)
        onManifestRequest func(c *Client, msg *Message, p ManifestRequestPayload)
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

func (h *Handler) OnChunkRequest(fn func(c *Client, msg *Message, p ChunkRequestPayload)) {
	h.onChunkRequest = fn
}

func (h *Handler) OnManifestRequest(fn func(c *Client, msg *Message, p ManifestRequestPayload)) {
	h.onManifestRequest = fn
}
func (h *Handler) OnChunkResponse(fn func(c *Client, p ChunkResponsePayload)) { h.onChunkResponse = fn }
func (h *Handler) OnChunkHave(fn func(c *Client, p ChunkHavePayload))         { h.onChunkHave = fn }
func (h *Handler) OnPeerAnnounce(fn func(c *Client, p PeerAnnouncePayload))   { h.onPeerAnnounce = fn }
func (h *Handler) OnHandshake(fn func(c *Client, p HandshakePayload))         { h.onHandshake = fn }
func (h *Handler) OnDHTMessage(fn func(c *Client, msg *Message))             { h.onDHTMessage = fn }

func (h *Handler) Dispatch(c *Client, msg *Message) {
	// Resolve dulu di level generic — ini nyakup DHT response DAN chunk
	// pull response, satu tracker buat semua request/response pattern.
	// Kalau msg ini balesan buat request yang lagi ditunggu, langsung
	// selesai di sini, gak lanjut ke switch di bawah.
	if h.hub.RPC().Resolve(msg) {
		return
	}

	if isDHTMessage(msg.Type) {
		if h.onDHTMessage != nil {
			h.onDHTMessage(c, msg)
		}
		return
	}

	switch msg.Type {
	case MsgTypeHandshake:
		var p HandshakePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad handshake payload from %s: %v", c.PeerID, err)
			return
		}
		if p.PeerID != "" && p.PeerID != c.PeerID {
			h.hub.Rekey(c.PeerID, p.PeerID)
		}
		c.PublicKey = p.PublicKey
		if h.onHandshake != nil {
			h.onHandshake(c, p)
		}

	case MsgTypePing:
		h.handlePing(c)

	case MsgTypePeerAnnounce:
		var p PeerAnnouncePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad peer_announce payload: %v", err)
			return
		}
		if h.onPeerAnnounce != nil {
			h.onPeerAnnounce(c, p)
		}

	case MsgTypeChunkRequest:
		var p ChunkRequestPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad chunk_request payload: %v", err)
			return
		}
		if h.onChunkRequest != nil {
			h.onChunkRequest(c, msg, p) // msg diteruskan biar ID-nya bisa di-echo di response
		}

	case MsgTypeChunkResponse:
		// ini cuma nyampe sini kalau ID-nya gak match pending RPC apapun
		// (misal push yang gak diminta) — tetep di-log biar kelihatan.
		var p ChunkResponsePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad chunk_response payload: %v", err)
			return
		}
		if h.onChunkResponse != nil {
			h.onChunkResponse(c, p)
		}

	case MsgTypeChunkHave:
		var p ChunkHavePayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad chunk_have payload: %v", err)
			return
		}
		if h.onChunkHave != nil {
			h.onChunkHave(c, p)
		}

	case MsgTypeDirectMessage:
		utils.LogInfo("direct message from %s", c.PeerID)

        case MsgTypeManifestRequest:
		var p ManifestRequestPayload
		if err := json.Unmarshal(msg.Payload, &p); err != nil {
			utils.LogWarn("bad manifest_request payload: %v", err)
			return
		}
		if h.onManifestRequest != nil {
			h.onManifestRequest(c, msg, p)
		}

	case MsgTypeManifestResponse:
		// only reaches here if it didn't match a pending RPC (already
		// resolved earlier in Dispatch via hub.RPC().Resolve) — log as
		// a diagnostic, nothing to do with an unsolicited manifest.
		utils.LogWarn("received unsolicited manifest_response from %s", c.PeerID)

	default:
		utils.LogWarn("unknown message type from %s: %s", c.PeerID, msg.Type)
	}
}

func isDHTMessage(t MessageType) bool {
	switch t {
	case MsgTypeDHTFindNode, MsgTypeDHTFindNodeResp, MsgTypeDHTStore, MsgTypeDHTFindValue, MsgTypeDHTFindValueResp:
		return true
	}
	return false
}

func (h *Handler) handlePing(c *Client) {
	pong := &Message{Type: MsgTypePong, Timestamp: time.Now().Unix()}
	c.SendMessage(pong)
}
