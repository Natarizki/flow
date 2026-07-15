package websocket

import "encoding/json"

type MessageType string

const (
	MsgTypeHandshake     MessageType = "handshake"
	MsgTypePeerAnnounce  MessageType = "peer_announce"
	MsgTypePeerList      MessageType = "peer_list"
	MsgTypeChunkRequest  MessageType = "chunk_request"
	MsgTypeChunkResponse MessageType = "chunk_response"
	MsgTypeChunkHave     MessageType = "chunk_have"
	MsgTypePing          MessageType = "ping"
	MsgTypePong          MessageType = "pong"
	MsgTypeError         MessageType = "error"
	MsgTypeDirectMessage MessageType = "direct_message"

	// Kademlia DHT protocol messages
	MsgTypeDHTFindNode      MessageType = "dht_find_node"
	MsgTypeDHTFindNodeResp  MessageType = "dht_find_node_resp"
	MsgTypeDHTStore         MessageType = "dht_store"
	MsgTypeDHTFindValue     MessageType = "dht_find_value"
	MsgTypeDHTFindValueResp MessageType = "dht_find_value_resp"
        MsgTypeManifestRequest  MessageType = "manifest_request"
	MsgTypeManifestResponse MessageType = "manifest_response"

)

// Message.ID dipakai buat correlate request/response pada RPC DHT (async
// message-passing butuh cara nyocokin balesan ke request yang bener).
type Message struct {
	Type      MessageType     `json:"type"`
	ID        string          `json:"id,omitempty"`
	PeerID    string          `json:"peer_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

type HandshakePayload struct {
	PeerID     string `json:"peer_id"`
	PublicKey  string `json:"public_key"`
	Version    string `json:"version"`
	ClientType string `json:"client_type"`
}

type PeerAnnouncePayload struct {
	PeerID  string   `json:"peer_id"`
	Address string   `json:"address"`
	Port    int      `json:"port"`
	Tags    []string `json:"tags,omitempty"`
	Visible string   `json:"visible"`
}

type ChunkRequestPayload struct {
	ChunkHash string `json:"chunk_hash"`
	FileHash  string `json:"file_hash"`
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
}

type ChunkResponsePayload struct {
	ChunkHash string `json:"chunk_hash"`
	Data      []byte `json:"data"`
	Checksum  string `json:"checksum"`
}

type ChunkHavePayload struct {
	FileHash    string   `json:"file_hash"`
	ChunkHashes []string `json:"chunk_hashes"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DHTContact adalah representasi 1 node buat dikirim lewat wire (bukan
// pakai struct p2p.Contact langsung biar websocket package gak circular
// import ke p2p package).
type DHTContact struct {
	ID      string `json:"id"`      // hex-encoded NodeID
	Address string `json:"address"` // ws:// url buat dial balik
}

type DHTFindNodePayload struct {
	TargetID string `json:"target_id"`
}

type DHTFindNodeRespPayload struct {
	Contacts []DHTContact `json:"contacts"`
}

type DHTStorePayload struct {
	Key     string `json:"key"`
	Value   string `json:"value"` // peer ID pemilik konten
	TTLSecs int    `json:"ttl_secs"`
}

type DHTFindValuePayload struct {
	Key string `json:"key"`
}

type DHTFindValueRespPayload struct {
	Found    bool         `json:"found"`
	Values   []string     `json:"values,omitempty"`   // kalau found: peer ID providers
	Contacts []DHTContact `json:"contacts,omitempty"` // kalau !found: node terdekat buat lanjut lookup
}

type ManifestRequestPayload struct {
	FileHash string `json:"file_hash"`
}

type ManifestResponsePayload struct {
	FileHash    string   `json:"file_hash"`
	ChunkHashes []string `json:"chunk_hashes"`
	Found       bool     `json:"found"`
}

func NewMessage(msgType MessageType, peerID string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:    msgType,
		PeerID:  peerID,
		Payload: data,
	}, nil
}

func NewMessageWithID(msgType MessageType, id, peerID string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:    msgType,
		ID:      id,
		PeerID:  peerID,
		Payload: data,
	}, nil
}
