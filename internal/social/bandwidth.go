package social

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type BandwidthRecord struct {
	Date        string `json:"date"` // "2026-07-13"
	BytesUp     int64  `json:"bytes_up"`
	BytesDown   int64  `json:"bytes_down"`
}

// BandwidthTracker keeps real daily counters of bytes served (up) and
// bytes pulled from other peers (down), persisted per calendar day so
// `flc bandwidth --today` / `--month` reflect actual transferred bytes,
// not estimates.
type BandwidthTracker struct {
	records map[string]*BandwidthRecord // date -> record
	mu      sync.Mutex
	store   *store.Store
}

func NewBandwidthTracker(st *store.Store) *BandwidthTracker {
	b := &BandwidthTracker{records: make(map[string]*BandwidthRecord), store: st}
	if st != nil {
		b.loadFromStore()
	}
	return b
}

func (b *BandwidthTracker) loadFromStore() {
	entries, err := b.store.List("bandwidth:")
	if err != nil {
		utils.LogWarn("failed to load bandwidth records: %v", err)
		return
	}
	for _, raw := range entries {
		var r BandwidthRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		b.records[r.Date] = &r
	}
}

func (b *BandwidthTracker) persist(r *BandwidthRecord) {
	if b.store == nil {
		return
	}
	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	b.store.Set("bandwidth:"+r.Date, data)
}

func todayKey() string {
	return time.Now().UTC().Format("2006-01-02")
}

func (b *BandwidthTracker) RecordUpload(bytes int64) {
	date := todayKey()
	b.mu.Lock()
	r, ok := b.records[date]
	if !ok {
		r = &BandwidthRecord{Date: date}
		b.records[date] = r
	}
	r.BytesUp += bytes
	b.mu.Unlock()
	b.persist(r)
}

func (b *BandwidthTracker) RecordDownload(bytes int64) {
	date := todayKey()
	b.mu.Lock()
	r, ok := b.records[date]
	if !ok {
		r = &BandwidthRecord{Date: date}
		b.records[date] = r
	}
	r.BytesDown += bytes
	b.mu.Unlock()
	b.persist(r)
}

func (b *BandwidthTracker) Today() *BandwidthRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.records[todayKey()]; ok {
		copy := *r
		return &copy
	}
	return &BandwidthRecord{Date: todayKey()}
}

func (b *BandwidthTracker) ThisMonth() *BandwidthRecord {
	prefix := time.Now().UTC().Format("2006-01")
	b.mu.Lock()
	defer b.mu.Unlock()

	total := &BandwidthRecord{Date: prefix}
	for date, r := range b.records {
		if len(date) >= 7 && date[:7] == prefix {
			total.BytesUp += r.BytesUp
			total.BytesDown += r.BytesDown
		}
	}
	return total
}
