package cache

import (
	"sync"
	"time"
)

// IncognitoIndex is a drop-in alternative to Index that never touches
// disk or the persistent metadata store — everything lives in memory
// and is discarded when the process exits or the session ends. Real
// incognito behavior means nothing about visited URLs, cache contents,
// or access patterns survives a restart, not just "hidden from the UI".
type IncognitoIndex struct {
	entries map[string]*IndexEntry
	mu      sync.RWMutex
	maxSize int64
	current int64
}

func NewIncognitoIndex(maxSize int64) *IncognitoIndex {
	return &IncognitoIndex{
		entries: make(map[string]*IndexEntry),
		maxSize: maxSize,
	}
}

func (idx *IncognitoIndex) Put(entry *IndexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entry.CreatedAt = time.Now()
	entry.LastAccess = time.Now()
	idx.entries[entry.Hash] = entry
	idx.current += entry.Size

	// simple eviction: if over budget, drop oldest entries until under
	for idx.current > idx.maxSize && len(idx.entries) > 0 {
		var oldestHash string
		var oldestTime time.Time
		first := true
		for h, e := range idx.entries {
			if first || e.LastAccess.Before(oldestTime) {
				oldestHash, oldestTime = h, e.LastAccess
				first = false
			}
		}
		if oldestHash == "" {
			break
		}
		idx.current -= idx.entries[oldestHash].Size
		delete(idx.entries, oldestHash)
	}
}

func (idx *IncognitoIndex) Get(hash string) (*IndexEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	e, ok := idx.entries[hash]
	return e, ok
}

func (idx *IncognitoIndex) Stats() (count int, totalSize int64) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries), idx.current
}

// Purge wipes everything immediately — called explicitly when
// incognito mode ends, in addition to the natural process-exit wipe.
func (idx *IncognitoIndex) Purge() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.entries = make(map[string]*IndexEntry)
	idx.current = 0
}
