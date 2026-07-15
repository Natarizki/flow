package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Natarizki/flow/pkg/utils"
)

const (
	dialTimeout       = 10 * time.Second
	reconnectBaseWait = 2 * time.Second
	reconnectMaxWait  = 60 * time.Second
)

// DialPeer connect keluar (sebagai client) ke url WSS peer/tracker lain,
// kirim handshake, terus daftarin koneksi itu ke Hub yang sama seperti
// koneksi masuk. Dari sudut pandang Hub, gak ada beda antara peer yang
// connect ke kita vs peer yang kita connect ke — keduanya jadi *Client.
func DialPeer(hub *Hub, handler *Handler, url string, selfPeerID, selfPubKey string) (*Client, error) {
	dialer := websocket.Dialer{HandshakeTimeout: dialTimeout}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return nil, utils.WrapError("DIAL_FAILED", fmt.Sprintf("failed to dial %s", url), err)
	}

	handshake := HandshakePayload{
		PeerID:     selfPeerID,
		PublicKey:  selfPubKey,
		Version:    "0.1.0",
		ClientType: "flow-daemon",
	}
	msg, err := NewMessage(MsgTypeHandshake, selfPeerID, handshake)
	if err != nil {
		conn.Close()
		return nil, err
	}
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return nil, utils.WrapError("DIAL_FAILED", "handshake write failed", err)
	}

	// remote peer ID belum kita tau sampe dia balas handshake-nya sendiri;
	// sementara pakai url sebagai temporary key, di-update pas handshake
	// response masuk lewat handler biasa.
	client := NewClient(url, conn, hub)
	hub.Register(client)

	go client.ReadPump(handler)
	go client.WritePump()

	utils.LogInfo("dialed peer at %s", url)
	return client, nil
}

// AutoDial coba connect ke url dengan retry + exponential backoff,
// jalan terus di background sampe berhasil atau di-stop lewat stopCh.
// Dipanggil sebagai goroutine terpisah per target address.
func AutoDial(hub *Hub, handler *Handler, url, selfPeerID, selfPubKey string, stopCh <-chan struct{}) {
	wait := reconnectBaseWait

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		client, err := DialPeer(hub, handler, url, selfPeerID, selfPubKey)
		if err != nil {
			utils.LogWarn("auto-dial to %s failed: %v, retrying in %s", url, err, wait)
			select {
			case <-time.After(wait):
			case <-stopCh:
				return
			}
			wait *= 2
			if wait > reconnectMaxWait {
				wait = reconnectMaxWait
			}
			continue
		}

		wait = reconnectBaseWait

		// blok di sini sampe koneksi putus (ReadPump keluar dan
		// unregister client dari hub), baru retry
		waitUntilDisconnected(hub, client)

		select {
		case <-stopCh:
			return
		case <-time.After(wait):
		}
	}
}

// AutoDialWithPriority behaves like AutoDial, but when priority is true
// (the target is in the same enterprise Mesh as us), it uses a much
// shorter base reconnect backoff — same-mesh peers are assumed to be
// organizationally important to stay connected to, so we retry harder
// and faster rather than backing off to the full 60s ceiling.
func AutoDialWithPriority(hub *Hub, handler *Handler, url, selfPeerID, selfPubKey string, priority bool, stopCh <-chan struct{}) {
	wait := reconnectBaseWait
	maxWait := reconnectMaxWait
	if priority {
		wait = 500 * time.Millisecond
		maxWait = 5 * time.Second
	}

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		client, err := DialPeer(hub, handler, url, selfPeerID, selfPubKey)
		if err != nil {
			utils.LogWarn("auto-dial to %s failed: %v, retrying in %s", url, err, wait)
			select {
			case <-time.After(wait):
			case <-stopCh:
				return
			}
			wait *= 2
			if wait > maxWait {
				wait = maxWait
			}
			continue
		}

		if priority {
			wait = 500 * time.Millisecond
		} else {
			wait = reconnectBaseWait
		}

		waitUntilDisconnected(hub, client)

		select {
		case <-stopCh:
			return
		case <-time.After(wait):
		}
	}
}

func waitUntilDisconnected(hub *Hub, client *Client) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if _, ok := hub.GetClient(client.PeerID); !ok {
			return
		}
	}
}
