package api

import (
	"net/http"
	"time"

	"github.com/Natarizki/flow/internal/cache"
)

func secondsToDuration(sec int) time.Duration {
	return time.Duration(sec) * time.Second
}

// asPersistentIndex tries to get the concrete *cache.Index out of the
// IndexStore interface — List/Clean/Export/Import only make sense for
// the disk-backed index. In incognito mode s.index is *cache.IncognitoIndex
// instead, and these operations correctly refuse rather than silently
// no-op.
func (s *Server) asPersistentIndex() (*cache.Index, bool) {
	idx, ok := s.index.(*cache.Index)
	return idx, ok
}

func (s *Server) handleCacheList(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.asPersistentIndex()
	if !ok {
		writeError(w, http.StatusForbidden, "cache listing is unavailable in incognito mode")
		return
	}
	writeJSON(w, http.StatusOK, idx.List())
}

func (s *Server) handleCacheClean(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	idx, ok := s.asPersistentIndex()
	if !ok {
		writeError(w, http.StatusForbidden, "cache cleaning is unavailable in incognito mode")
		return
	}

	var req struct {
		OlderThanSeconds int `json:"older_than_seconds"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	removed := idx.CleanOlderThan(secondsToDuration(req.OlderThanSeconds))
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

func (s *Server) handleCacheExport(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.asPersistentIndex()
	if !ok {
		writeError(w, http.StatusForbidden, "cache export is unavailable in incognito mode")
		return
	}

	var req struct {
		Destination string `json:"destination"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := idx.ExportArchive(req.Destination); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "exported"})
}

func (s *Server) handleCacheImport(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.asPersistentIndex()
	if !ok {
		writeError(w, http.StatusForbidden, "cache import is unavailable in incognito mode")
		return
	}

	var req struct {
		Source string `json:"source"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	count, err := idx.ImportArchive(req.Source)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "imported", "count": count})
}

func (s *Server) handleCacheRead(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		writeError(w, http.StatusBadRequest, "hash query param required")
		return
	}
	data, err := s.fetcher.ReadDecoded(hash)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Write(data)
}
