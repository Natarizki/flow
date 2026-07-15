package api

import (
	"github.com/Natarizki/flow/internal/auth"
)

// loginUser & refreshUserToken di file terpisah biar auth_handlers.go
// fokus ke HTTP concerns, bukan business logic auth itu sendiri.

func loginUser(s *Server, email, password string) (*auth.LoginResult, error) {
	result, err := auth.Login(s.users, email, password)
	if err != nil {
		return nil, err
	}
	s.sessions.SetActive(result.User.ID, result.Token)
	return result, nil
}

func refreshUserToken(claims *auth.Claims) (string, error) {
	return auth.GenerateToken(claims.UserID, claims.Username, claims.Email)
}
