package api

import (
	"net/http"

	"github.com/Natarizki/flow/internal/social"
)

func (s *Server) handleVideoPrerollPage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HTMLHash string `json:"html_hash"`
		BaseURL  string `json:"base_url"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	data, err := s.fetcher.ReadDecoded(req.HTMLHash)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	count := s.videoPreroller.PrerollAllInPage(data, req.BaseURL)
	writeJSON(w, http.StatusOK, map[string]int{"prerolled": count})
}

func (s *Server) handleBookmarkAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string   `json:"url"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	bm := s.bookmarks.Add(req.URL, req.Title, req.Tags)
	writeJSON(w, http.StatusCreated, bm)
}

func (s *Server) handleBookmarkRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.bookmarks.Remove(req.ID) {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) handleBookmarkList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.bookmarks.List())
}

func (s *Server) handleBookmarkSyncExport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.bookmarks.ExportAll())
}

func (s *Server) handleBookmarkSyncMerge(w http.ResponseWriter, r *http.Request) {
	var incoming []*social.Bookmark
	if err := decodeBody(r, &incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid bookmark payload")
		return
	}
	changed := s.bookmarks.MergeFrom(incoming)
	writeJSON(w, http.StatusOK, map[string]int{"changed": changed})
}

func (s *Server) handleWikipediaPrecache(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lang string `json:"lang"`
	}
	decodeBody(r, &req)
	count, err := s.wikiPrecacher.PrecacheMostRead(req.Lang)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"cached": count})
}
