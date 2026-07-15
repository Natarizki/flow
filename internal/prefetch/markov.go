package prefetch

import (
	"sync"
)

// MarkovChain model transisi "dari halaman A, user paling sering pindah
// ke halaman apa" — order-1 (cuma liat 1 langkah sebelumnya). Cukup buat
// prediksi next-page tanpa perlu training berat.
type MarkovChain struct {
	transitions map[string]map[string]int // from -> to -> count
	totalFrom   map[string]int            // from -> total transisi keluar
	mu          sync.RWMutex
}

func NewMarkovChain() *MarkovChain {
	return &MarkovChain{
		transitions: make(map[string]map[string]int),
		totalFrom:   make(map[string]int),
	}
}

// Train ambil urutan URL dari 1 sesi browsing dan catat semua transisi
// A->B di dalamnya.
func (m *MarkovChain) Train(sequence []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := 0; i < len(sequence)-1; i++ {
		from, to := sequence[i], sequence[i+1]
		if from == to {
			continue
		}
		if _, ok := m.transitions[from]; !ok {
			m.transitions[from] = make(map[string]int)
		}
		m.transitions[from][to]++
		m.totalFrom[from]++
	}
}

// Observe catat 1 transisi tunggal secara live (dipanggil tiap kali user
// beneran pindah halaman, bukan cuma dari training batch).
func (m *MarkovChain) Observe(from, to string) {
	if from == "" || from == to {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.transitions[from]; !ok {
		m.transitions[from] = make(map[string]int)
	}
	m.transitions[from][to]++
	m.totalFrom[from]++
}

type Prediction struct {
	URL         string  `json:"url"`
	Probability float64 `json:"probability"`
}

// Predict kembalikan top-N kandidat halaman selanjutnya dari `from`,
// diurutkan dari probabilitas tertinggi.
func (m *MarkovChain) Predict(from string, topN int) []Prediction {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dests, ok := m.transitions[from]
	if !ok {
		return nil
	}
	total := m.totalFrom[from]
	if total == 0 {
		return nil
	}

	predictions := make([]Prediction, 0, len(dests))
	for url, count := range dests {
		predictions = append(predictions, Prediction{
			URL:         url,
			Probability: float64(count) / float64(total),
		})
	}

	// sort descending by probability (insertion sort, cukup buat topN kecil)
	for i := 1; i < len(predictions); i++ {
		for j := i; j > 0 && predictions[j].Probability > predictions[j-1].Probability; j-- {
			predictions[j], predictions[j-1] = predictions[j-1], predictions[j]
		}
	}

	if topN > 0 && len(predictions) > topN {
		predictions = predictions[:topN]
	}
	return predictions
}

func (m *MarkovChain) TransitionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, dests := range m.transitions {
		total += len(dests)
	}
	return total
}
