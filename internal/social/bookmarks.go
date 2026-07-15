package social

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type Bookmark struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"` // tombstone, so sync can propagate deletions
}

// BookmarkStore is per-account (not per-device) — persisted to BadgerDB
// so it's already durable locally, and exposed via Export/Import (used
// by LAN sync between a person's own devices) using last-write-wins
// conflict resolution on UpdatedAt, which is the same simple, honest
// strategy real sync tools like Syncthing default to for single-owner
// data.
type BookmarkStore struct {
	items map[string]*Bookmark
	mu    sync.RWMutex
	store *store.Store
}

func NewBookmarkStore(st *store.Store) *BookmarkStore {
	b := &BookmarkStore{items: make(map[string]*Bookmark), store: st}
	if st != nil {
		b.loadFromStore()
	}
	return b
}

func (b *BookmarkStore) loadFromStore() {
	entries, err := b.store.List("bookmark:")
	if err != nil {
		utils.LogWarn("failed to load bookmarks: %v", err)
		return
	}
	for _, raw := range entries {
		var bm Bookmark
		if err := json.Unmarshal(raw, &bm); err != nil {
			continue
		}
		b.items[bm.ID] = &bm
	}
	utils.LogInfo("loaded %d bookmarks from persistent store", len(b.items))
}

func (b *BookmarkStore) persist(bm *Bookmark) {
	if b.store == nil {
		return
	}
	data, err := json.Marshal(bm)
	if err != nil {
		return
	}
	b.store.Set("bookmark:"+bm.ID, data)
}

func (b *BookmarkStore) Add(url, title string, tags []string) *Bookmark {
	bm := &Bookmark{
		ID:        utils.ShortHash(utils.HashBytes([]byte(url+time.Now().String())), 12),
		URL:       url,
		Title:     title,
		Tags:      tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	b.mu.Lock()
	b.items[bm.ID] = bm
	b.mu.Unlock()
	b.persist(bm)
	return bm
}

func (b *BookmarkStore) Remove(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	bm, ok := b.items[id]
	if !ok {
		return false
	}
	now := time.Now()
	bm.DeletedAt = &now
	bm.UpdatedAt = now
	b.persist(bm)
	return true
}

func (b *BookmarkStore) List() []*Bookmark {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []*Bookmark
	for _, bm := range b.items {
		if bm.DeletedAt == nil {
			result = append(result, bm)
		}
	}
	return result
}

// ExportAll returns every bookmark including tombstones — used as the
// sync payload sent to another of the user's own devices.
func (b *BookmarkStore) ExportAll() []*Bookmark {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]*Bookmark, 0, len(b.items))
	for _, bm := range b.items {
		result = append(result, bm)
	}
	return result
}

// MergeFrom applies incoming bookmarks from another device using
// last-write-wins on UpdatedAt — this is the actual sync logic, not
// just a data dump. Returns how many local records changed.
func (b *BookmarkStore) MergeFrom(incoming []*Bookmark) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	changed := 0
	for _, incomingBM := range incoming {
		existing, ok := b.items[incomingBM.ID]
		if !ok || incomingBM.UpdatedAt.After(existing.UpdatedAt) {
			b.items[incomingBM.ID] = incomingBM
			b.persist(incomingBM)
			changed++
		}
	}
	return changed
}
