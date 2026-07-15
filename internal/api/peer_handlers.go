package api

import (
	"net/http"
	"strings"

	"github.com/Natarizki/flow/internal/p2p"
	websocketpkg "github.com/Natarizki/flow/internal/websocket"
)

type createPeerRequest struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

func (s *Server) handlePeersCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.peers.List())

	case http.MethodPost:
		var req createPeerRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "peer name is required")
			return
		}
		if req.Visibility == "" {
			req.Visibility = "private"
		}

		peer := &p2p.Peer{
			ID:         generatePeerID(req.Name),
			Name:       req.Name,
			Visibility: p2p.PeerVisibility(req.Visibility),
		}
		s.peers.Add(peer)
		writeJSON(w, http.StatusCreated, peer)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handlePeerItem menangani semua path /api/peers/<name>[/action] karena
// stdlib ServeMux (Go 1.21) belum ada path param matching.
func (s *Server) handlePeerItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/peers/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "peer name is required")
		return
	}
	name := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	peer, ok := findPeerByName(s.peers, name)
	if !ok && action != "rename" {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}

	switch action {
	case "":
		writeJSON(w, http.StatusOK, peer)

	case "delete":
		s.peers.Remove(peer.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	case "rename":
		var req struct {
			NewName string `json:"new_name"`
		}
		decodeBody(r, &req)
		if !s.peers.Rename(peer.ID, req.NewName) {
			writeError(w, http.StatusNotFound, "peer not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})

	case "visibility":
		var req struct {
			Visibility string `json:"visibility"`
		}
		decodeBody(r, &req)
		s.peers.SetVisibility(peer.ID, p2p.PeerVisibility(req.Visibility))
		writeJSON(w, http.StatusOK, map[string]string{"status": "visibility updated"})

	case "lock":
		var req struct {
			Message string `json:"message"`
		}
		decodeBody(r, &req)
		s.peers.Lock(peer.ID, req.Message)
		writeJSON(w, http.StatusOK, map[string]string{"status": "locked"})

	case "unlock":
		s.peers.Unlock(peer.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "unlocked"})

	case "message":
		var req struct {
			Message string `json:"message"`
		}
		decodeBody(r, &req)
		client, ok := s.hub.GetClient(peer.ID)
		if !ok {
			writeError(w, http.StatusNotFound, "peer not connected")
			return
		}
		msg, err := websocketpkg.NewMessage(websocketpkg.MsgTypeDirectMessage, "", req.Message)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build message")
			return
		}
		client.SendMessage(msg)
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})

	case "readme":
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]string{
				"readme":        peer.Readme,
				"readme_format": peer.ReadmeFormat,
			})

		case http.MethodPost:
			var req struct {
				Content string `json:"content"`
				Format  string `json:"format"`
			}
			if err := decodeBody(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			if !s.peers.SetReadme(peer.ID, req.Content, req.Format) {
				writeError(w, http.StatusNotFound, "peer not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "readme updated"})

		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	default:
		writeError(w, http.StatusNotFound, "unknown action")
	}
}

func findPeerByName(pm *p2p.PeerManager, name string) (*p2p.Peer, bool) {
	for _, p := range pm.List() {
		if p.Name == name || p.ID == name {
			return p, true
		}
	}
	return nil, false
}

func generatePeerID(name string) string {
	return "peer-" + name
}
