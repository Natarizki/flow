package prefetch

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/Natarizki/flow/pkg/utils"
)

// HistoryEntry format input buat `flc prefetch train --history <file.json>`
type HistoryEntry struct {
	SessionID string   `json:"session_id"`
	Sequence  []string `json:"sequence"`
}

// Predictor bungkus MarkovChain + state enable/disable + tracking URL
// terakhir yang diakses per "sesi" (per peer/browser), biar tau harus
// prediksi dari mana.
type Predictor struct {
	chain    *MarkovChain
	enabled  bool
	lastSeen map[string]string // sessionID -> last URL
	mu       sync.RWMutex
}

func NewPredictor() *Predictor {
	return &Predictor{
		chain:    NewMarkovChain(),
		enabled:  true,
		lastSeen: make(map[string]string),
	}
}

func (p *Predictor) Enable()  { p.mu.Lock(); p.enabled = true; p.mu.Unlock() }
func (p *Predictor) Disable() { p.mu.Lock(); p.enabled = false; p.mu.Unlock() }

func (p *Predictor) IsEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled
}

// TrainFromFile baca file JSON berisi array HistoryEntry dan train
// MarkovChain dari semua sequence di dalamnya.
func (p *Predictor) TrainFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, utils.WrapError("PREFETCH_TRAIN", "failed to read history file", err)
	}

	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, utils.WrapError("PREFETCH_TRAIN", "failed to parse history JSON", err)
	}

	for _, e := range entries {
		p.chain.Train(e.Sequence)
	}

	utils.LogInfo("prefetch: trained on %d sessions", len(entries))
	return len(entries), nil
}

// RecordVisit dipanggil setiap kali ada request nyata masuk (bukan
// prefetch), buat live-update Markov chain sekaligus track posisi
// terakhir per sesi.
func (p *Predictor) RecordVisit(sessionID, url string) {
	if !p.IsEnabled() {
		return
	}
	p.mu.Lock()
	prevURL := p.lastSeen[sessionID]
	p.lastSeen[sessionID] = url
	p.mu.Unlock()

	if prevURL != "" {
		p.chain.Observe(prevURL, url)
	}
}

// Predict prediksi topN URL berikutnya dari suatu URL tertentu.
func (p *Predictor) Predict(fromURL string, topN int) []Prediction {
	if !p.IsEnabled() {
		return nil
	}
	return p.chain.Predict(fromURL, topN)
}

// PredictForSession prediksi berdasarkan posisi terakhir sesi tertentu.
func (p *Predictor) PredictForSession(sessionID string, topN int) []Prediction {
	p.mu.RLock()
	lastURL := p.lastSeen[sessionID]
	p.mu.RUnlock()

	if lastURL == "" {
		return nil
	}
	return p.Predict(lastURL, topN)
}

// ActiveSessions balikin semua sessionID yang punya minimal 1 recorded
// visit. Dipakai Scheduler.RunBackground buat tau sesi mana aja yang
// perlu dicek prediksinya — sebelumnya callback ini selalu dikasih slice
// kosong dari main.go, jadi background prefetch efektif no-op.
func (p *Predictor) ActiveSessions() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sessions := make([]string, 0, len(p.lastSeen))
	for sid := range p.lastSeen {
		sessions = append(sessions, sid)
	}
	return sessions
}

func (p *Predictor) TransitionCount() int {
	return p.chain.TransitionCount()
}
