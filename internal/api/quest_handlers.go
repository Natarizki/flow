package api

import "net/http"

func (s *Server) handleQuestsToday(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.quests.TodaysQuests())
}

func (s *Server) handleQuestsProgress(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("peer_id")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer_id query param required")
		return
	}
	writeJSON(w, http.StatusOK, s.quests.ProgressFor(peerID))
}
