package auth

import (
	"encoding/json"
	"regexp"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
}

// UserStore sekarang persist ke BadgerDB kalau store dikasih (non-nil).
// Kalau nil, tetap jalan in-memory doang (dipakai buat testing).
type UserStore struct {
	byID    map[string]*User
	byEmail map[string]*User
	mu      sync.RWMutex
	store   *store.Store
}

func NewUserStore(st *store.Store) *UserStore {
	s := &UserStore{
		byID:    make(map[string]*User),
		byEmail: make(map[string]*User),
		store:   st,
	}
	if st != nil {
		s.loadFromStore()
	}
	return s
}

func (s *UserStore) loadFromStore() {
	entries, err := s.store.List("user:")
	if err != nil {
		utils.LogWarn("failed to load users from store: %v", err)
		return
	}
	for _, raw := range entries {
		var u User
		if err := json.Unmarshal(raw, &u); err != nil {
			utils.LogWarn("skipping corrupt user record: %v", err)
			continue
		}
		s.byID[u.ID] = &u
		s.byEmail[u.Email] = &u
	}
	utils.LogInfo("loaded %d users from persistent store", len(s.byID))
}

func (s *UserStore) persist(u *User) {
	if s.store == nil {
		return
	}
	data, err := json.Marshal(u)
	if err != nil {
		utils.LogWarn("failed to marshal user %s: %v", u.ID, err)
		return
	}
	if err := s.store.Set("user:"+u.ID, data); err != nil {
		utils.LogWarn("failed to persist user %s: %v", u.ID, err)
	}
}

func (s *UserStore) Register(email, username, password string) (*User, error) {
	if !emailRe.MatchString(email) {
		return nil, utils.NewError("INVALID_EMAIL", "email format is invalid")
	}
	if len(username) < 3 {
		return nil, utils.NewError("INVALID_USERNAME", "username must be at least 3 characters")
	}
	if len(password) < 8 {
		return nil, utils.NewError("WEAK_PASSWORD", "password must be at least 8 characters")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byEmail[email]; exists {
		return nil, utils.NewError("EMAIL_TAKEN", "email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, utils.WrapError("HASH_ERROR", "failed to hash password", err)
	}

	user := &User{
		ID:           utils.ShortHash(utils.HashBytes([]byte(email+username)), 16),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
	}

	s.byID[user.ID] = user
	s.byEmail[email] = user
	s.persist(user)

	return user, nil
}

func (s *UserStore) GetByEmail(email string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byEmail[email]
	return u, ok
}

func (s *UserStore) GetByID(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byID[id]
	return u, ok
}
