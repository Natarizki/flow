package enterprise

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

// Snapshot adalah satu titik data time-series — direkam periodik (misal
// tiap jam) biar Analytics Report bisa nunjukin tren, bukan cuma angka
// sesaat kayak /api/stats biasa.
type Snapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	PeerCount    int       `json:"peer_count"`
	CacheEntries int       `json:"cache_entries"`
	CacheSize    int64     `json:"cache_size"`
	BytesServed  int64     `json:"bytes_served"`
	ChunksServed int64     `json:"chunks_served"`
}

// Analytics simpen time-series snapshot ke BadgerDB dan bisa export
// jadi CSV ("Analytics Report (CSV)" di daftar fitur).
type Analytics struct {
	store *store.Store
	mu    sync.Mutex
}

func NewAnalytics(st *store.Store) *Analytics {
	return &Analytics{store: st}
}

func (a *Analytics) RecordSnapshot(s Snapshot) {
	if a.store == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := json.Marshal(s)
	if err != nil {
		utils.LogWarn("analytics: failed to marshal snapshot: %v", err)
		return
	}
	key := "analytics:" + s.Timestamp.Format(time.RFC3339)
	if err := a.store.Set(key, data); err != nil {
		utils.LogWarn("analytics: failed to persist snapshot: %v", err)
	}
}

// RunPeriodicSnapshot jalan sebagai goroutine background, ambil snapshot
// tiap interval dari fungsi collector yang diinject (biar package ini
// gak perlu tau detail hub/index/leaderboard langsung).
func (a *Analytics) RunPeriodicSnapshot(collector func() Snapshot, interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			a.RecordSnapshot(collector())
		}
	}
}

func (a *Analytics) List(since time.Time) ([]Snapshot, error) {
	if a.store == nil {
		return nil, nil
	}
	entries, err := a.store.List("analytics:")
	if err != nil {
		return nil, utils.WrapError("ANALYTICS_LIST", "failed to list snapshots", err)
	}

	result := make([]Snapshot, 0, len(entries))
	for _, raw := range entries {
		var s Snapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if s.Timestamp.After(since) {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result, nil
}

// ExportCSV nulis semua snapshot (since waktu tertentu) sebagai CSV ke
// writer manapun (http.ResponseWriter, file, dll).
func (a *Analytics) ExportCSV(w io.Writer, since time.Time) error {
	snapshots, err := a.List(since)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{"timestamp", "peer_count", "cache_entries", "cache_size", "bytes_served", "chunks_served"})
	for _, s := range snapshots {
		cw.Write([]string{
			s.Timestamp.Format(time.RFC3339),
			itoa(s.PeerCount),
			itoa(s.CacheEntries),
			itoa64(s.CacheSize),
			itoa64(s.BytesServed),
			itoa64(s.ChunksServed),
		})
	}
	return nil
}

func itoa(n int) string   { return itoa64(int64(n)) }
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
