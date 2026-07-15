package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/Natarizki/flow/pkg/utils"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// FLOW's /ws endpoint is meant for peer-to-peer daemon
		// connections (other FLOW instances dialing in), not browser
		// clients — browsers send an Origin header, daemon-to-daemon
		// WebSocket dials (via gorilla's Dialer, used in dialer.go)
		// don't. So: no Origin header present = trust it (it's another
		// FLOW daemon or a non-browser client); Origin header present =
		// it's a browser, and we only allow same-origin requests to it
		// (matching the Host header), rather than blindly accepting
		// upgrade requests from any arbitrary website's JS.
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return isSameOrigin(origin, r.Host)
	},
}

func isSameOrigin(origin, host string) bool {
	// origin looks like "http://example.com:1234" or "https://..." —
	// compare just the host:port part against r.Host
	for _, prefix := range []string{"http://", "https://"} {
		if len(origin) > len(prefix) && origin[:len(prefix)] == prefix {
			return origin[len(prefix):] == host
		}
	}
	return false
}

func ServeWS(hub *Hub, handler *Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			utils.LogError("websocket upgrade failed: %v", err)
			return
		}

		tempID := conn.RemoteAddr().String()
		client := NewClient(tempID, conn, hub)
		hub.Register(client)

		go client.ReadPump(handler)
		go client.WritePump()

		utils.LogInfo("incoming peer connection from %s", tempID)
	}
}
