package cache

import "sync"

// IncognitoStorage is an in-memory stand-in for Storage — used together
// with IncognitoIndex so an incognito session writes literally zero
// bytes to disk. Implements the same three methods Fetcher calls
// (Write/Read/Delete), so it can be swapped in via an interface too.
type IncognitoStorage struct {
	blobs map[string][]byte
	mu    sync.RWMutex
}

func NewIncognitoStorage() *IncognitoStorage {
	return &IncognitoStorage{blobs: make(map[string][]byte)}
}

func (s *IncognitoStorage) Write(hash string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[hash] = append([]byte{}, data...)
	return nil
}

func (s *IncognitoStorage) Read(hash string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.blobs[hash]
	if !ok {
		return nil, ErrNotFoundIncognito
	}
	return data, nil
}

func (s *IncognitoStorage) Delete(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blobs, hash)
	return nil
}

func (s *IncognitoStorage) Purge() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs = make(map[string][]byte)
}

var ErrNotFoundIncognito = &incognitoNotFoundErr{}

type incognitoNotFoundErr struct{}

func (e *incognitoNotFoundErr) Error() string { return "not found (incognito storage)" }
