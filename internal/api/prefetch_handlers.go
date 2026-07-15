package api

import (
	"net/http"

	"github.com/Natarizki/flow/internal/prefetch"
)

func (s *Server) handlePrefetchTrain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HistoryFile string `json:"history_file"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sessions, err := s.predictor.TrainFromFile(req.HistoryFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sessions": sessions})
}

func (s *Server) handlePrefetchPredict(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, http.StatusBadRequest, "url query param required")
		return
	}
	predictions := s.predictor.Predict(url, 5)
	writeJSON(w, http.StatusOK, predictions)
}

func (s *Server) handlePrefetchEnable(w http.ResponseWriter, r *http.Request) {
	s.predictor.Enable()
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

func (s *Server) handlePrefetchDisable(w http.ResponseWriter, r *http.Request) {
	s.predictor.Disable()
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (s *Server) handlePrefetchNow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Depth int    `json:"depth"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Depth <= 0 {
		req.Depth = 3
	}

	count := s.scheduler.PrefetchNow(req.URL, req.Depth)
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (s *Server) handlePrefetchRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		URL       string `json:"url"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "session_id and url are required")
		return
	}

	s.predictor.RecordVisit(req.SessionID, req.URL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

var _ = prefetch.Prediction{} // silence unused import kalau predictions dipakai lewat s.predictor
