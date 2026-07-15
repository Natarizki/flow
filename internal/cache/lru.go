package cache

import (
	"container/list"
	"sync"
	"time"
)

type LRUEntry struct {
	Key       string
	Size      int64
	LastUsed  time.Time
	listElem  *list.Element
}

type LRU struct {
	maxSize     int64
	currentSize int64
	entries     map[string]*LRUEntry
	order       *list.List
	mu          sync.Mutex
	onEvict     func(key string)
}

func NewLRU(maxSize int64) *LRU {
	return &LRU{
		maxSize: maxSize,
		entries: make(map[string]*LRUEntry),
		order:   list.New(),
	}
}

func (l *LRU) OnEvict(fn func(key string)) {
	l.onEvict = fn
}

func (l *LRU) Touch(key string, size int64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.entries[key]; ok {
		l.currentSize += size - entry.Size
		entry.Size = size
		entry.LastUsed = time.Now()
		l.order.MoveToFront(entry.listElem)
	} else {
		elem := l.order.PushFront(key)
		entry := &LRUEntry{
			Key:      key,
			Size:     size,
			LastUsed: time.Now(),
			listElem: elem,
		}
		l.entries[key] = entry
		l.currentSize += size
	}

	l.evictIfNeeded()
}

func (l *LRU) Remove(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, ok := l.entries[key]; ok {
		l.order.Remove(entry.listElem)
		l.currentSize -= entry.Size
		delete(l.entries, key)
	}
}

func (l *LRU) evictIfNeeded() {
	for l.currentSize > l.maxSize && l.order.Len() > 0 {
		back := l.order.Back()
		if back == nil {
			break
		}
		key := back.Value.(string)
		entry := l.entries[key]

		l.order.Remove(back)
		l.currentSize -= entry.Size
		delete(l.entries, key)

		if l.onEvict != nil {
			l.onEvict(key)
		}
	}
}

func (l *LRU) CurrentSize() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.currentSize
}

func (l *LRU) MaxSize() int64 {
	return l.maxSize
}

func (l *LRU) SetMaxSize(size int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxSize = size
	l.evictIfNeeded()
}

func (l *LRU) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

func (l *LRU) StaleEntries(olderThan time.Duration) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var stale []string
	for key, entry := range l.entries {
		if entry.LastUsed.Before(cutoff) {
			stale = append(stale, key)
		}
	}
	return stale
}
