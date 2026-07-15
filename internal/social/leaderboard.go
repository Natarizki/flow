package social

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type LeaderboardEntry struct {
	PeerID       string `json:"peer_id"`
	Username     string `json:"username,omitempty"`
	BytesServed  int64  `json:"bytes_served"`
	ChunksServed int64  `json:"chunks_served"`
	Score        int64  `json:"score"` // derived metric, bukan cuma bytes mentah
}

// Leaderboard nge-track kontribusi beneran: berapa byte/chunk yang
// dilayani suatu peer ke peer lain. Setiap kali daemon berhasil ngirim
// chunk_response ke peer lain, RecordContribution dipanggil — bukan
// angka fiktif.
type Leaderboard struct {
	entries map[string]*LeaderboardEntry
	mu      sync.RWMutex
	store   *store.Store
}

func NewLeaderboard(st *store.Store) *Leaderboard {
	l := &Leaderboard{
		entries: make(map[string]*LeaderboardEntry),
		store:   st,
	}
	if st != nil {
		l.loadFromStore()
	}
	return l
}

func (l *Leaderboard) loadFromStore() {
	entries, err := l.store.List("leaderboard:")
	if err != nil {
		utils.LogWarn("failed to load leaderboard from store: %v", err)
		return
	}
	for _, raw := range entries {
		var e LeaderboardEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		l.entries[e.PeerID] = &e
	}
	utils.LogInfo("loaded %d leaderboard entries from persistent store", len(l.entries))
}

func (l *Leaderboard) persist(e *LeaderboardEntry) {
	if l.store == nil {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	l.store.Set("leaderboard:"+e.PeerID, data)
}

// RecordContribution dipanggil tiap kali peer ini beneran ngirim data ke
// peer lain (chunk_response sukses terkirim). Score = bytes/1024 + chunks*10,
// biar peer yang ngelayani banyak chunk kecil juga dapet kredit, bukan
// cuma yang ngirim file gede sekali doang.
func (l *Leaderboard) RecordContribution(peerID, username string, bytesServed int64) {
	l.mu.Lock()
	e, ok := l.entries[peerID]
	if !ok {
		e = &LeaderboardEntry{PeerID: peerID, Username: username}
		l.entries[peerID] = e
	}
	if username != "" {
		e.Username = username
	}
	e.BytesServed += bytesServed
	e.ChunksServed++
	e.Score = e.BytesServed/1024 + e.ChunksServed*10
	l.mu.Unlock()

	l.persist(e)
}

func (l *Leaderboard) GetGlobal(limit int) []*LeaderboardEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*LeaderboardEntry, 0, len(l.entries))
	for _, e := range l.entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Score > result[j].Score })

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func (l *Leaderboard) GetRank(peerID string) (int, *LeaderboardEntry) {
	all := l.GetGlobal(0)
	for i, e := range all {
		if e.PeerID == peerID {
			return i + 1, e
		}
	}
	return 0, nil
}
