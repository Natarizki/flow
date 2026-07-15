package api

import "net/http"

func (s *Server) handleDNSResolve(w http.ResponseWriter, r *http.Request) {
	hostname := r.URL.Query().Get("host")
	if hostname == "" {
		writeError(w, http.StatusBadRequest, "host query param required")
		return
	}
	ips, err := s.dns.Resolve(hostname)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"host": hostname, "ips": ips})
}
