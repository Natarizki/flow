package p2p

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type PeerVisibility string

const (
	VisibilityPublic   PeerVisibility = "public"
	VisibilityPrivate  PeerVisibility = "private"
	VisibilityInternal PeerVisibility = "internal"
)

type Peer struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Address    string         `json:"address"`
	Port       int            `json:"port"`
	PublicKey  string         `json:"public_key"`
	Visibility PeerVisibility `json:"visibility"`
	Tags       []string       `json:"tags"`
	Reputation float64        `json:"reputation"`
	LastSeen   time.Time      `json:"last_seen"`
	Locked     bool           `json:"locked"`
	LockMsg    string         `json:"lock_message,omitempty"`
	Readme     string         `json:"readme,omitempty"`        // raw markdown or mdx source
	ReadmeFormat string       `json:"readme_format,omitempty"` // "md" or "mdx"
}

type PeerManager struct {
	peers map[string]*Peer
	mu    sync.RWMutex
	store *store.Store
}

func NewPeerManager(st *store.Store) *PeerManager {
	pm := &PeerManager{
		peers: make(map[string]*Peer),
		store: st,
	}
	if st != nil {
		pm.loadFromStore()
	}
	return pm
}

func (pm *PeerManager) loadFromStore() {
	entries, err := pm.store.List("peer:")
	if err != nil {
		utils.LogWarn("failed to load peers from store: %v", err)
		return
	}
	for _, raw := range entries {
		var p Peer
		if err := json.Unmarshal(raw, &p); err != nil {
			utils.LogWarn("skipping corrupt peer record: %v", err)
			continue
		}
		pm.peers[p.ID] = &p
	}
	utils.LogInfo("loaded %d peers from persistent store", len(pm.peers))
}

func (pm *PeerManager) persist(p *Peer) {
	if pm.store == nil {
		return
	}
	data, err := json.Marshal(p)
	if err != nil {
		utils.LogWarn("failed to marshal peer %s: %v", p.ID, err)
		return
	}
	if err := pm.store.Set("peer:"+p.ID, data); err != nil {
		utils.LogWarn("failed to persist peer %s: %v", p.ID, err)
	}
}

func (pm *PeerManager) Add(p *Peer) {
	pm.mu.Lock()
	p.LastSeen = time.Now()
	pm.peers[p.ID] = p
	pm.mu.Unlock()
	pm.persist(p)
}

func (pm *PeerManager) Get(id string) (*Peer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.peers[id]
	return p, ok
}

func (pm *PeerManager) Remove(id string) {
	pm.mu.Lock()
	delete(pm.peers, id)
	pm.mu.Unlock()
	if pm.store != nil {
		if err := pm.store.Delete("peer:" + id); err != nil {
			utils.LogWarn("failed to delete persisted peer %s: %v", id, err)
		}
	}
}

func (pm *PeerManager) Rename(id, newName string) bool {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Name = newName
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
	return ok
}

func (pm *PeerManager) SetVisibility(id string, v PeerVisibility) bool {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Visibility = v
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
	return ok
}

func (pm *PeerManager) Lock(id, message string) bool {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Locked = true
		p.LockMsg = message
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
	return ok
}

func (pm *PeerManager) Unlock(id string) bool {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Locked = false
		p.LockMsg = ""
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
	return ok
}

func (pm *PeerManager) List() []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]*Peer, 0, len(pm.peers))
	for _, p := range pm.peers {
		result = append(result, p)
	}
	return result
}

func (pm *PeerManager) UpdateReputation(id string, delta float64) {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Reputation += delta
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
}

func (pm *PeerManager) SetReadme(id, content, format string) bool {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	if ok {
		p.Readme = content
		if format == "mdx" {
			p.ReadmeFormat = "mdx"
		} else {
			p.ReadmeFormat = "md"
		}
	}
	pm.mu.Unlock()
	if ok {
		pm.persist(p)
	}
	return ok
}
