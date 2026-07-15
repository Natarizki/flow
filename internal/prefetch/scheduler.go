package prefetch

import (
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// FetchFunc adalah callback yang beneran ngambil+cache konten dari URL.
// Diinject dari luar (main.go) biar package prefetch gak perlu tau detail
// HTTP client / cache writer.
type FetchFunc func(url string) error

// Scheduler jalanin prefetch berdasarkan prediksi Markov chain, dengan
// depth-limited BFS: prefetch top prediction, terus dari situ prefetch
// lagi top prediction-nya, sampe kedalaman tertentu.
type Scheduler struct {
	predictor *Predictor
	fetch     FetchFunc
	minProb   float64 // ambang minimum probability biar prefetch worth it

	mu      sync.Mutex
	pending map[string]bool // url -> sedang di-prefetch, cegah duplikat
}

func NewScheduler(predictor *Predictor, fetch FetchFunc) *Scheduler {
	return &Scheduler{
		predictor: predictor,
		fetch:     fetch,
		minProb:   0.15,
		pending:   make(map[string]bool),
	}
}

func (s *Scheduler) SetMinProbability(p float64) {
	s.minProb = p
}

// PrefetchNow trigger prefetch langsung dari satu URL, sampai depth N.
// Ini yang dipanggil dari `flc prefetch now <url> --depth 3`.
func (s *Scheduler) PrefetchNow(startURL string, depth int) int {
	visited := make(map[string]bool)
	count := s.prefetchRecursive(startURL, depth, visited)
	utils.LogInfo("prefetch: fetched %d pages starting from %s (depth %d)", count, startURL, depth)
	return count
}

func (s *Scheduler) prefetchRecursive(fromURL string, depth int, visited map[string]bool) int {
	if depth <= 0 || visited[fromURL] {
		return 0
	}
	visited[fromURL] = true

	predictions := s.predictor.Predict(fromURL, 5)
	count := 0

	for _, pred := range predictions {
		if pred.Probability < s.minProb || visited[pred.URL] {
			continue
		}

		if s.tryFetch(pred.URL) {
			count++
		}
		count += s.prefetchRecursive(pred.URL, depth-1, visited)
	}

	return count
}

func (s *Scheduler) tryFetch(url string) bool {
	s.mu.Lock()
	if s.pending[url] {
		s.mu.Unlock()
		return false
	}
	s.pending[url] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, url)
		s.mu.Unlock()
	}()

	if err := s.fetch(url); err != nil {
		utils.LogWarn("prefetch failed for %s: %v", url, err)
		return false
	}
	return true
}

// RunPeakHourAware jalan sebagai goroutine background, cek prediksi buat
// semua sesi aktif secara periodik, dan prefetch otomatis kalau
// probabilitasnya cukup tinggi. Dipanggil dari main.go kalau
// `prefetch enable` aktif.
func (s *Scheduler) RunBackground(sessionIDs func() []string, interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if !s.predictor.IsEnabled() {
				continue
			}
			for _, sid := range sessionIDs() {
				predictions := s.predictor.PredictForSession(sid, 3)
				for _, pred := range predictions {
					if pred.Probability >= s.minProb {
						go s.tryFetch(pred.URL)
					}
				}
			}
		}
	}
}
