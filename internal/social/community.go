package social

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type CommunityEvent struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Participants []string `json:"participants"`
}

// CommunityManager ngelola event komunitas (misal "seeding weekend" —
// double score buat siapapun yang aktif ngelayani chunk selama periode
// tertentu). Event dibuat manual lewat API/CLI admin, bukan otomatis.
type CommunityManager struct {
	events map[string]*CommunityEvent
	mu     sync.RWMutex
	store  *store.Store
}

func NewCommunityManager(st *store.Store) *CommunityManager {
	c := &CommunityManager{
		events: make(map[string]*CommunityEvent),
		store:  st,
	}
	if st != nil {
		c.loadFromStore()
	}
	return c
}

func (c *CommunityManager) loadFromStore() {
	entries, err := c.store.List("event:")
	if err != nil {
		utils.LogWarn("failed to load community events from store: %v", err)
		return
	}
	for _, raw := range entries {
		var e CommunityEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		c.events[e.ID] = &e
	}
	utils.LogInfo("loaded %d community events from persistent store", len(c.events))
}

func (c *CommunityManager) persist(e *CommunityEvent) {
	if c.store == nil {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	c.store.Set("event:"+e.ID, data)
}

func (c *CommunityManager) CreateEvent(title, description string, start, end time.Time) *CommunityEvent {
	e := &CommunityEvent{
		ID:          utils.ShortHash(utils.HashBytes([]byte(title+start.String())), 12),
		Title:       title,
		Description: description,
		StartTime:   start,
		EndTime:     end,
	}

	c.mu.Lock()
	c.events[e.ID] = e
	c.mu.Unlock()

	c.persist(e)
	return e
}

func (c *CommunityManager) Join(eventID, peerID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.events[eventID]
	if !ok {
		return false
	}
	for _, p := range e.Participants {
		if p == peerID {
			return true // udah join
		}
	}
	e.Participants = append(e.Participants, peerID)
	c.persist(e)
	return true
}

func (c *CommunityManager) List() []*CommunityEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*CommunityEvent, 0, len(c.events))
	for _, e := range c.events {
		result = append(result, e)
	}
	return result
}

func (c *CommunityManager) Active() []*CommunityEvent {
	now := time.Now()
	var active []*CommunityEvent
	for _, e := range c.List() {
		if now.After(e.StartTime) && now.Before(e.EndTime) {
			active = append(active, e)
		}
	}
	return active
}
