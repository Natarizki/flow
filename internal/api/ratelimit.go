package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginRateLimiter is a simple sliding-window limiter keyed by client
// IP: N failed attempts within a window triggers a temporary lockout.
// This is intentionally in-memory (not persisted) — a restart resets
// limits, which is an acceptable tradeoff for a self-hosted daemon
// (the alternative, persisting attacker IPs forever, isn't obviously
// better and adds complexity for little benefit here).
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	maxTries int
	window   time.Duration
	lockout  time.Duration
	lockedAt map[string]time.Time
}

func newLoginRateLimiter(maxTries int, window, lockout time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string][]time.Time),
		lockedAt: make(map[string]time.Time),
		maxTries: maxTries,
		window:   window,
		lockout:  lockout,
	}
}

func clientIP(r *http.Request) string {
	// honor X-Forwarded-For if present (behind a reverse proxy), else
	// use the raw remote address
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Allowed checks if this IP is currently permitted to attempt a login.
func (l *loginRateLimiter) Allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if lockedAt, ok := l.lockedAt[ip]; ok {
		if time.Since(lockedAt) < l.lockout {
			return false
		}
		delete(l.lockedAt, ip)
		delete(l.attempts, ip)
	}
	return true
}

// RecordFailure logs a failed attempt and locks the IP out if it's hit
// the threshold within the current window.
func (l *loginRateLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	recent := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	l.attempts[ip] = recent

	if len(recent) >= l.maxTries {
		l.lockedAt[ip] = now
	}
}

// RecordSuccess clears any tracked failures for this IP — a successful
// login resets the counter rather than leaving stale near-miss attempts
// lingering.
func (l *loginRateLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
	delete(l.lockedAt, ip)
}
