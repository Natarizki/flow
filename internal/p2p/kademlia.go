package p2p

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

const (
	IDBits   = 256 // pakai SHA-256 full, bukan 160-bit SHA-1 klasik
	NumBucket = IDBits
	BucketK  = 20 // ukuran max tiap k-bucket, standar Kademlia
	Alpha    = 3  // paralelisme query per iterasi lookup
)

type NodeID [32]byte

func NewNodeID(seed string) NodeID {
	h := sha256.Sum256([]byte(seed))
	return NodeID(h)
}

// NewNodeIDFromPubKey hashes the raw Ed25519 public key bytes directly,
// rather than hashing the hex-string representation of the fingerprint
// (which NewNodeID does). This is the more correct approach: identical
// key material always produces the identical NodeID without going
// through an intermediate string encoding step, and it avoids the (tiny
// but real) risk of hex-encoding differences producing different IDs
// for the same key.
func NewNodeIDFromPubKey(pubKey []byte) NodeID {
	return sha256.Sum256(pubKey)
}

func NodeIDFromHex(s string) (NodeID, bool) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return NodeID{}, false
	}
	var id NodeID
	copy(id[:], b)
	return id, true
}

func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

// Distance = XOR metric, inti dari Kademlia. Semakin kecil hasil XOR
// (dibaca sebagai integer besar), semakin "dekat" dua node.
func Distance(a, b NodeID) NodeID {
	var d NodeID
	for i := range a {
		d[i] = a[i] ^ b[i]
	}
	return d
}

// Less bandingin dua NodeID sebagai integer besar (big-endian), dipakai
// buat sorting by distance.
func (id NodeID) Less(other NodeID) bool {
	for i := range id {
		if id[i] != other[i] {
			return id[i] < other[i]
		}
	}
	return false
}

// bucketIndex hitung index k-bucket berdasarkan posisi bit pertama yang
// beda antara self dan target (common prefix length).
func bucketIndex(self, target NodeID) int {
	d := Distance(self, target)
	for i := 0; i < len(d); i++ {
		if d[i] == 0 {
			continue
		}
		for bit := 0; bit < 8; bit++ {
			if d[i]&(0x80>>bit) != 0 {
				return i*8 + bit
			}
		}
	}
	return IDBits - 1 // identical (self lookup), taruh di bucket terjauh
}

type Contact struct {
	ID       NodeID
	Address  string // ws:// url buat dial balik
	LastSeen time.Time
}

type kBucket struct {
	contacts []Contact
}

func (b *kBucket) upsert(c Contact) {
	for i, existing := range b.contacts {
		if existing.ID == c.ID {
			b.contacts[i] = c // refresh posisi & LastSeen
			return
		}
	}
	if len(b.contacts) < BucketK {
		b.contacts = append(b.contacts, c)
		return
	}
	// bucket penuh: replace kontak paling lama gak keliatan (simple
	// eviction, bukan least-recently-seen strict Kademlia ping-check,
	// cukup buat skala P2P kecil-menengah)
	oldestIdx := 0
	for i, existing := range b.contacts {
		if existing.LastSeen.Before(b.contacts[oldestIdx].LastSeen) {
			oldestIdx = i
		}
	}
	b.contacts[oldestIdx] = c
}

// RoutingTable adalah tabel k-bucket standar Kademlia: 256 bucket (satu
// per bit posisi), tiap bucket nyimpen sampe BucketK kontak yang jaraknya
// (dari self) jatuh di rentang bit itu.
type RoutingTable struct {
	self    NodeID
	buckets [NumBucket]*kBucket
	mu      sync.RWMutex
}

func NewRoutingTable(self NodeID) *RoutingTable {
	rt := &RoutingTable{self: self}
	for i := range rt.buckets {
		rt.buckets[i] = &kBucket{}
	}
	return rt
}

func (rt *RoutingTable) Update(c Contact) {
	if c.ID == rt.self {
		return
	}
	c.LastSeen = time.Now()
	idx := bucketIndex(rt.self, c.ID)

	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.buckets[idx].upsert(c)
}

func (rt *RoutingTable) Remove(id NodeID) {
	idx := bucketIndex(rt.self, id)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	b := rt.buckets[idx]
	for i, c := range b.contacts {
		if c.ID == id {
			b.contacts = append(b.contacts[:i], b.contacts[i+1:]...)
			return
		}
	}
}

// Closest balikin `count` kontak paling deket ke target, diurutkan
// terdekat dulu. Ini fungsi paling penting buat iterative lookup.
func (rt *RoutingTable) Closest(target NodeID, count int) []Contact {
	rt.mu.RLock()
	all := make([]Contact, 0, BucketK*4)
	for _, b := range rt.buckets {
		all = append(all, b.contacts...)
	}
	rt.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool {
		return Distance(all[i].ID, target).Less(Distance(all[j].ID, target))
	})

	if len(all) > count {
		all = all[:count]
	}
	return all
}

func (rt *RoutingTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	total := 0
	for _, b := range rt.buckets {
		total += len(b.contacts)
	}
	return total
}

func (rt *RoutingTable) All() []Contact {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	all := make([]Contact, 0)
	for _, b := range rt.buckets {
		all = append(all, b.contacts...)
	}
	return all
}
