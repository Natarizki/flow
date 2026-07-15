package api

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleLeaderboardGlobal(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, s.leaderboard.GetGlobal(limit))
}

func (s *Server) handleAchievementsList(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("peer_id")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer_id query param required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unlocked": s.achievements.ListUnlocked(peerID),
		"catalog":  BadgeCatalogRef(),
	})
}

func (s *Server) handleCommunityEventsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.community.List())
}

func (s *Server) handleCommunityEventCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_time, use RFC3339 format")
		return
	}
	end, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_time, use RFC3339 format")
		return
	}

	event := s.community.CreateEvent(req.Title, req.Description, start, end)
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) handleCommunityEventJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID string `json:"event_id"`
		PeerID  string `json:"peer_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.community.Join(req.EventID, req.PeerID) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}
