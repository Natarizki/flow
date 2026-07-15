package security

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// PhishingBlacklist checks domains against a local hash-based blocklist.
// Domains are stored as SHA-256 hashes (not plaintext) so the blocklist
// file itself doesn't become a directory of "here are known-bad sites"
// readable by anyone browsing the cache dir — matches on hash lookup.
type PhishingBlacklist struct {
	hashes map[string]bool
	mu     sync.RWMutex
	path   string
}

func NewPhishingBlacklist(path string) *PhishingBlacklist {
	pb := &PhishingBlacklist{hashes: make(map[string]bool), path: path}
	pb.LoadFromFile(path)
	return pb
}

func domainHash(domain string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(domain))))
	return hex.EncodeToString(h[:])
}

// LoadFromFile reads a plaintext list of domains (one per line, '#'
// comments allowed) and hashes them into memory. This is the on-disk
// format an admin edits; internally we only keep hashes.
func (pb *PhishingBlacklist) LoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			utils.LogWarn("phishing blacklist file not found at %s, starting empty", path)
			return nil
		}
		return utils.WrapError("PHISHING_LOAD", "failed to open blacklist file", err)
	}
	defer f.Close()

	pb.mu.Lock()
	defer pb.mu.Unlock()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pb.hashes[domainHash(line)] = true
		count++
	}
	utils.LogInfo("loaded %d phishing blacklist entries from %s", count, path)
	return nil
}

// FetchRemoteList downloads a plaintext domain list from a URL (e.g. a
// community-maintained blocklist) and merges it into the local set,
// appending new entries to the on-disk file so they persist.
func (pb *PhishingBlacklist) FetchRemoteList(listURL string) (int, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(listURL)
	if err != nil {
		return 0, utils.WrapError("PHISHING_FETCH", "failed to fetch remote blocklist", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, utils.NewError("PHISHING_FETCH_HTTP", "remote blocklist returned error status")
	}

	pb.mu.Lock()
	defer pb.mu.Unlock()

	added := 0
	scanner := bufio.NewScanner(resp.Body)
	var newDomains []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		h := domainHash(line)
		if !pb.hashes[h] {
			pb.hashes[h] = true
			newDomains = append(newDomains, line)
			added++
		}
	}

	if added > 0 && pb.path != "" {
		f, err := os.OpenFile(pb.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			for _, d := range newDomains {
				f.WriteString(d + "\n")
			}
		}
	}

	utils.LogInfo("fetched remote phishing blocklist: %d new entries added", added)
	return added, nil
}

// IsBlocked checks a URL's host against the blacklist.
func (pb *PhishingBlacklist) IsBlocked(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}

	pb.mu.RLock()
	defer pb.mu.RUnlock()

	// check full host and progressively shorter parent domains
	// (e.g. "evil.sub.example.com" also matches a blocked "example.com")
	parts := strings.Split(host, ".")
	for i := 0; i < len(parts)-1; i++ {
		candidate := strings.Join(parts[i:], ".")
		if pb.hashes[domainHash(candidate)] {
			return true
		}
	}
	return false
}

func (pb *PhishingBlacklist) Count() int {
	pb.mu.RLock()
	defer pb.mu.RUnlock()
	return len(pb.hashes)
}

func (pb *PhishingBlacklist) AddDomain(domain string) {
	pb.mu.Lock()
	pb.hashes[domainHash(domain)] = true
	pb.mu.Unlock()

	if pb.path != "" {
		if f, err := os.OpenFile(pb.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			defer f.Close()
			f.WriteString(domain + "\n")
		}
	}
}
