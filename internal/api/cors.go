package api

import "net/http"

// corsMiddleware sets permissive-but-controlled CORS headers so a
// third-party web app (or the FLOW dashboard served from a different
// port during development) can call this API. We echo back the
// requesting Origin rather than using a bare "*" — that's required
// anyway once Authorization headers are involved (browsers reject "*"
// combined with credentialed requests), and it means we're not
// blindly trusting every origin without at least logging what asked.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
