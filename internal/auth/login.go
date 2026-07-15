package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type LoginResult struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

func Login(userStore *UserStore, email, password string) (*LoginResult, error) {
	user, ok := userStore.GetByEmail(email)
	if !ok {
		return nil, utils.ErrAuthFailed
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, utils.ErrAuthFailed
	}

	token, err := GenerateToken(user.ID, user.Username, user.Email)
	if err != nil {
		return nil, err
	}

	return &LoginResult{Token: token, User: user}, nil
}

// SessionStore sekarang persist token aktif ke BadgerDB juga, jadi kalau
// daemon restart, session yang masih valid gak langsung ke-invalidate
// paksa (walau JWT-nya sendiri tetap punya expiry independen).
type SessionStore struct {
	activeTokens map[string]string // userID -> current token
	mu           sync.RWMutex
	store        *store.Store
}

func NewSessionStore(st *store.Store) *SessionStore {
	s := &SessionStore{
		activeTokens: make(map[string]string),
		store:        st,
	}
	if st != nil {
		s.loadFromStore()
	}
	return s
}

func (s *SessionStore) loadFromStore() {
	entries, err := s.store.List("session:")
	if err != nil {
		utils.LogWarn("failed to load sessions from store: %v", err)
		return
	}
	for key, val := range entries {
		userID := key[len("session:"):]
		s.activeTokens[userID] = string(val)
	}
	utils.LogInfo("loaded %d active sessions from persistent store", len(s.activeTokens))
}

func (s *SessionStore) SetActive(userID, token string) {
	s.mu.Lock()
	s.activeTokens[userID] = token
	s.mu.Unlock()

	if s.store != nil {
		if err := s.store.Set("session:"+userID, []byte(token)); err != nil {
			utils.LogWarn("failed to persist session for %s: %v", userID, err)
		}
	}
}

func (s *SessionStore) IsActive(userID, token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active, ok := s.activeTokens[userID]
	return ok && active == token
}

func (s *SessionStore) Revoke(userID string) {
	s.mu.Lock()
	delete(s.activeTokens, userID)
	s.mu.Unlock()

	if s.store != nil {
		if err := s.store.Delete("session:" + userID); err != nil {
			utils.LogWarn("failed to delete persisted session for %s: %v", userID, err)
		}
	}
}

func Logout(sessions *SessionStore, userID string) {
	sessions.Revoke(userID)
}
