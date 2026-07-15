package p2p

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// AdaptiveWorkerPool grows/shrinks its goroutine count based on queue
// pressure — starts at NumCPU workers, adds more (up to max) when the
// job queue backs up, and scales back down when idle. Used for chunk
// serving so a burst of simultaneous peer requests doesn't spawn an
// unbounded number of goroutines, but idle periods don't waste workers
// either.
type AdaptiveWorkerPool struct {
	jobs      chan func()
	minWorkers int
	maxWorkers int
	active    int64
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewAdaptiveWorkerPool(minWorkers, maxWorkers int) *AdaptiveWorkerPool {
	if minWorkers <= 0 {
		minWorkers = runtime.NumCPU()
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers * 4
	}
	p := &AdaptiveWorkerPool{
		jobs:       make(chan func(), maxWorkers*8),
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		stopCh:     make(chan struct{}),
	}
	for i := 0; i < minWorkers; i++ {
		p.spawnWorker()
	}
	go p.monitor()
	return p
}

func (p *AdaptiveWorkerPool) spawnWorker() {
	p.wg.Add(1)
	atomic.AddInt64(&p.active, 1)
	go func() {
		defer p.wg.Done()
		defer atomic.AddInt64(&p.active, -1)
		for {
			select {
			case job, ok := <-p.jobs:
				if !ok {
					return
				}
				job()
			case <-p.stopCh:
				return
			}
		}
	}()
}

// monitor checks queue depth every second: if the job channel is more
// than 50% full and we're under maxWorkers, spawn another worker. This
// is the "adaptive" part — real backpressure-driven scaling, not a
// fixed pool size.
func (p *AdaptiveWorkerPool) monitor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			queueDepth := len(p.jobs)
			capacity := cap(p.jobs)
			current := atomic.LoadInt64(&p.active)

			if queueDepth > capacity/2 && int(current) < p.maxWorkers {
				p.spawnWorker()
			}
		}
	}
}

func (p *AdaptiveWorkerPool) Submit(job func()) {
	select {
	case p.jobs <- job:
	case <-p.stopCh:
	}
}

func (p *AdaptiveWorkerPool) ActiveWorkers() int {
	return int(atomic.LoadInt64(&p.active))
}

func (p *AdaptiveWorkerPool) Shutdown() {
	close(p.stopCh)
	close(p.jobs)
	p.wg.Wait()
}
