package api

import (
	"net/http"
	"time"

	"github.com/Natarizki/flow/internal/enterprise"
)

// requireEnterprise wrap handler, tolak akses kalau license aktif bukan
// tier enterprise. Fitur mesh controller & advanced analytics termasuk
// yang di-gate di sini (sesuai model bisnis freemium: 8 fitur ini butuh
// bayar).
func (s *Server) requireEnterprise(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.license.IsEnterprise() {
			writeError(w, http.StatusPaymentRequired, "this feature requires an active enterprise license")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleLicenseActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.license.Activate(req.Key); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.license.Current())
}

func (s *Server) handleLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if s.license.Current() == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"active": false, "tier": "free"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"active": true, "license": s.license.Current()})
}

func (s *Server) handleMeshCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		OrgName  string `json:"org_name"`
		Priority int    `json:"priority"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mesh := s.mesh.CreateMesh(req.ID, req.Name, req.OrgName, req.Priority)
	writeJSON(w, http.StatusCreated, mesh)
}

func (s *Server) handleMeshList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mesh.List())
}

func (s *Server) handleMeshAddMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MeshID string `json:"mesh_id"`
		PeerID string `json:"peer_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.mesh.AddMember(req.MeshID, req.PeerID) {
		writeError(w, http.StatusNotFound, "mesh not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *Server) handleAnalyticsList(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if parsed, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			since = parsed
		}
	}
	snapshots, err := s.analytics.List(since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) handleAnalyticsExportCSV(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if parsed, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			since = parsed
		}
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=flow-analytics.csv")
	if err := s.analytics.ExportCSV(w, since); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
	_ = enterprise.Snapshot{} // silence unused import if analytics type not referenced elsewhere here
}
