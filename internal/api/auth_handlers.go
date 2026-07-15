package api

import (
	"net/http"

	"github.com/Natarizki/flow/internal/auth"
)

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req registerRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.users.Register(req.Email, req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user": map[string]string{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ip := clientIP(r)
	if !s.loginLimiter.Allowed(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
		return
	}

	var req loginRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := loginUser(s, req.Email, req.Password)
	if err != nil {
		s.loginLimiter.RecordFailure(ip)
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.loginLimiter.RecordSuccess(ip)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": result.Token,
		"user": map[string]string{
			"id":       result.User.ID,
			"username": result.User.Username,
			"email":    result.User.Email,
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	s.sessions.Revoke(claims.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"email":    claims.Email,
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	token, err := refreshUserToken(claims)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}
