package p2p

import (
	"encoding/json"
	"sync"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

// TagManager tracks tag -> set of peer IDs, and provides reverse lookup
// (peer -> tags). Kept separate from Peer struct so tagging doesn't
// require rewriting the whole peer record on every tag op.
type TagManager struct {
	byTag  map[string]map[string]bool // tag -> set(peerID)
	byPeer map[string]map[string]bool // peerID -> set(tag)
	mu     sync.RWMutex
	store  *store.Store
}

func NewTagManager(st *store.Store) *TagManager {
	t := &TagManager{
		byTag:  make(map[string]map[string]bool),
		byPeer: make(map[string]map[string]bool),
		store:  st,
	}
	if st != nil {
		t.loadFromStore()
	}
	return t
}

type tagRecord struct {
	PeerID string   `json:"peer_id"`
	Tags   []string `json:"tags"`
}

func (t *TagManager) loadFromStore() {
	entries, err := t.store.List("tags:")
	if err != nil {
		utils.LogWarn("failed to load tags from store: %v", err)
		return
	}
	for _, raw := range entries {
		var rec tagRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		for _, tag := range rec.Tags {
			t.addLocal(rec.PeerID, tag)
		}
	}
}

func (t *TagManager) addLocal(peerID, tag string) {
	if t.byTag[tag] == nil {
		t.byTag[tag] = make(map[string]bool)
	}
	t.byTag[tag][peerID] = true
	if t.byPeer[peerID] == nil {
		t.byPeer[peerID] = make(map[string]bool)
	}
	t.byPeer[peerID][tag] = true
}

func (t *TagManager) persist(peerID string) {
	if t.store == nil {
		return
	}
	tags := t.TagsOf(peerID)
	data, err := json.Marshal(tagRecord{PeerID: peerID, Tags: tags})
	if err != nil {
		return
	}
	t.store.Set("tags:"+peerID, data)
}

func (t *TagManager) Add(peerID, tag string) {
	t.mu.Lock()
	t.addLocal(peerID, tag)
	t.mu.Unlock()
	t.persist(peerID)
}

func (t *TagManager) Remove(peerID, tag string) {
	t.mu.Lock()
	if t.byTag[tag] != nil {
		delete(t.byTag[tag], peerID)
	}
	if t.byPeer[peerID] != nil {
		delete(t.byPeer[peerID], tag)
	}
	t.mu.Unlock()
	t.persist(peerID)
}

func (t *TagManager) TagsOf(peerID string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var tags []string
	for tag := range t.byPeer[peerID] {
		tags = append(tags, tag)
	}
	return tags
}

func (t *TagManager) PeersWithTag(tag string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var peers []string
	for peerID := range t.byTag[tag] {
		peers = append(peers, peerID)
	}
	return peers
}
