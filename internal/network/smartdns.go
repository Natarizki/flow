package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// SmartDNS resolves hostnames via DNS-over-HTTPS (Cloudflare's
// 1.1.1.1/dns-query, using the JSON API) instead of the OS resolver —
// this is real DoH, encrypted end-to-end, with an in-memory TTL-respecting
// cache so repeated lookups for the same host during prefetch don't hit
// the network every time.
type SmartDNS struct {
	client   *http.Client
	cache    map[string]*dnsCacheEntry
	mu       sync.RWMutex
	endpoint string
}

type dnsCacheEntry struct {
	ips     []string
	expires time.Time
}

func NewSmartDNS() *SmartDNS {
	return &SmartDNS{
		client:   &http.Client{Timeout: 5 * time.Second},
		cache:    make(map[string]*dnsCacheEntry),
		endpoint: "https://cloudflare-dns.com/dns-query",
	}
}

type dohResponse struct {
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// Resolve returns A-record IPs for a hostname, using a DoH lookup and
// caching the result according to the TTL the DNS response itself
// specifies (not a fixed arbitrary cache time).
func (d *SmartDNS) Resolve(hostname string) ([]string, error) {
	d.mu.RLock()
	if entry, ok := d.cache[hostname]; ok && time.Now().Before(entry.expires) {
		ips := entry.ips
		d.mu.RUnlock()
		return ips, nil
	}
	d.mu.RUnlock()

	req, err := http.NewRequest("GET", d.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	q := req.URL.Query()
	q.Set("name", hostname)
	q.Set("type", "A")
	req.URL.RawQuery = q.Encode()

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, utils.WrapError("DOH_REQUEST", "DNS-over-HTTPS request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH resolver returned status %d", resp.StatusCode)
	}

	var parsed dohResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, utils.WrapError("DOH_PARSE", "failed to parse DoH response", err)
	}

	var ips []string
	minTTL := 3600
	for _, a := range parsed.Answer {
		if a.Type == 1 { // A record
			ips = append(ips, a.Data)
			if a.TTL < minTTL {
				minTTL = a.TTL
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no A records found for %s", hostname)
	}

	d.mu.Lock()
	d.cache[hostname] = &dnsCacheEntry{ips: ips, expires: time.Now().Add(time.Duration(minTTL) * time.Second)}
	d.mu.Unlock()

	return ips, nil
}

func (d *SmartDNS) CacheSize() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.cache)
}
