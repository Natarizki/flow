package p2p

import (
	"fmt"
	"sync"
	"time"

	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

type storedValue struct {
	providers map[string]time.Time // peerID -> expiry
}

// DHTNode implementasi Kademlia beneran: routing table k-bucket + RPC
// FIND_NODE/STORE/FIND_VALUE lewat WSS ke peer lain. Beda dari versi
// lama yang cuma map in-memory lokal doang dan gak pernah nanya peer
// lain apapun.
type DHTNode struct {
	self    NodeID
	table   *RoutingTable
	hub     *websocketpkg.Hub
	rpc     *websocketpkg.RPCTracker
	selfURL string

	values map[string]*storedValue // content hash -> providers (lokal + yang di-STORE ke kita)
	mu     sync.RWMutex
}

func NewDHTNode(selfPeerID, selfURL string, hub *websocketpkg.Hub) *DHTNode {
	return &DHTNode{
		self:    NewNodeID(selfPeerID),
		table:   NewRoutingTable(NewNodeID(selfPeerID)),
		hub:     hub,
		rpc:     hub.RPC(),
		selfURL: selfURL,
		values:  make(map[string]*storedValue),
	}
}

// NewDHTNodeFromPubKey builds a DHTNode whose identity is derived
// directly from raw Ed25519 public key bytes, which is what production
// code should use now — see NewNodeIDFromPubKey for why this is more
// correct than hashing a string fingerprint.
func NewDHTNodeFromPubKey(pubKey []byte, selfURL string, hub *websocketpkg.Hub) *DHTNode {
	id := NewNodeIDFromPubKey(pubKey)
	return &DHTNode{
		self:    id,
		table:   NewRoutingTable(id),
		hub:     hub,
		rpc:     hub.RPC(),
		selfURL: selfURL,
		values:  make(map[string]*storedValue),
	}
}

func (d *DHTNode) SelfID() NodeID { return d.self }

// HandleMessage dipanggil dari websocket.Handler tiap ada DHT message
// masuk — baik request (FIND_NODE/STORE/FIND_VALUE) yang perlu dibalas,
// maupun response yang perlu di-resolve ke pending RPC call.
func (d *DHTNode) HandleMessage(c *websocketpkg.Client, msg *websocketpkg.Message) {
	switch msg.Type {
	case websocketpkg.MsgTypeDHTFindNode:
		d.handleFindNode(c, msg)
	case websocketpkg.MsgTypeDHTStore:
		d.handleStore(c, msg)
	case websocketpkg.MsgTypeDHTFindValue:
		d.handleFindValue(c, msg)
	}
}

func (d *DHTNode) handleFindNode(c *websocketpkg.Client, msg *websocketpkg.Message) {
	var req websocketpkg.DHTFindNodePayload
	if parseErr := parsePayload(msg.Payload, &req); parseErr != nil {
		return
	}
	targetID, ok := NodeIDFromHex(req.TargetID)
	if !ok {
		return
	}

	closest := d.table.Closest(targetID, BucketK)
	resp, _ := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeDHTFindNodeResp, msg.ID, "", websocketpkg.DHTFindNodeRespPayload{
		Contacts: contactsToWire(closest),
	})
	c.SendMessage(resp)
}

func (d *DHTNode) handleStore(c *websocketpkg.Client, msg *websocketpkg.Message) {
	var req websocketpkg.DHTStorePayload
	if err := parsePayload(msg.Payload, &req); err != nil {
		return
	}
	ttl := time.Duration(req.TTLSecs) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	d.storeLocal(req.Key, req.Value, ttl)
}

func (d *DHTNode) handleFindValue(c *websocketpkg.Client, msg *websocketpkg.Message) {
	var req websocketpkg.DHTFindValuePayload
	if err := parsePayload(msg.Payload, &req); err != nil {
		return
	}

	if providers := d.localProviders(req.Key); len(providers) > 0 {
		resp, _ := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeDHTFindValueResp, msg.ID, "", websocketpkg.DHTFindValueRespPayload{
			Found:  true,
			Values: providers,
		})
		c.SendMessage(resp)
		return
	}

	targetID := NewNodeID(req.Key)
	closest := d.table.Closest(targetID, BucketK)
	resp, _ := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeDHTFindValueResp, msg.ID, "", websocketpkg.DHTFindValueRespPayload{
		Found:    false,
		Contacts: contactsToWire(closest),
	})
	c.SendMessage(resp)
}

// --- local storage helpers ---

func (d *DHTNode) storeLocal(key, providerID string, ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	v, ok := d.values[key]
	if !ok {
		v = &storedValue{providers: make(map[string]time.Time)}
		d.values[key] = v
	}
	v.providers[providerID] = time.Now().Add(ttl)
}

func (d *DHTNode) localProviders(key string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	v, ok := d.values[key]
	if !ok {
		return nil
	}
	now := time.Now()
	var out []string
	for peerID, expiry := range v.providers {
		if now.Before(expiry) {
			out = append(out, peerID)
		}
	}
	return out
}

// --- public API dipakai handler.OnChunkHave / fetcher / CLI ---

// RegisterContact masukin peer yang baru handshake ke routing table
// secara langsung (dipanggil dari OnHandshake, bukan lewat FIND_NODE
// response) — biar peer yang baru konek langsung dikenal tanpa nunggu
// lookup pertama.
func (d *DHTNode) RegisterContact(peerIDHex, address string) {
	id, ok := NodeIDFromHex(peerIDHex)
	if !ok {
		return
	}
	d.table.Update(Contact{ID: id, Address: address})
}

// RecordProvider catet peer sebagai provider buat key ini secara lokal
// aja (dari chunk_have announcement peer lain), tanpa trigger network
// STORE — dipakai buat optimistic caching dari gosip P2P biasa.
func (d *DHTNode) RecordProvider(key, peerID string) {
	d.storeLocal(key, peerID, 24*time.Hour)
}

// Provide announce diri sendiri sebagai provider buat key ini ke seluruh
// network: simpen lokal + iterative STORE ke BucketK node terdekat ke
// hash(key).
func (d *DHTNode) Provide(key string) {
	d.storeLocal(key, d.selfPeerIDString(), 24*time.Hour)

	targetID := NewNodeID(key)
	closest := d.iterativeFindNode(targetID)

	for _, contact := range closest {
		go d.sendStore(contact, key, d.selfPeerIDString())
	}
	utils.LogInfo("dht: announced self as provider for %s to %d nodes", utils.ShortHash(key, 8), len(closest))
}

// FindProviders cari peer mana yang punya konten ini — cek lokal dulu,
// kalau kosong baru iterative FIND_VALUE ke network.
func (d *DHTNode) FindProviders(key string) ([]string, error) {
	if local := d.localProviders(key); len(local) > 0 {
		return local, nil
	}

	targetID := NewNodeID(key)
	seen := make(map[NodeID]bool)
	shortlist := d.table.Closest(targetID, Alpha)

	for i := 0; i < 5 && len(shortlist) > 0; i++ { // max 5 iterasi lookup
		var next []Contact
		for _, c := range shortlist {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true

			values, contacts, err := d.queryFindValue(c, key)
			if err != nil {
				continue
			}
			if len(values) > 0 {
				return values, nil
			}
			for _, nc := range contacts {
				d.table.Update(nc)
				next = append(next, nc)
			}
		}
		if len(next) == 0 {
			break
		}
		shortlist = closestN(next, targetID, Alpha)
	}

	return nil, fmt.Errorf("dht: no providers found for %s", utils.ShortHash(key, 8))
}

// BootstrapFrom mulai kenal network dari satu contact awal (tracker) —
// query FIND_NODE buat diri sendiri, hasilnya ngisi routing table.
func (d *DHTNode) BootstrapFrom(contact Contact) {
	d.table.Update(contact)
	d.iterativeFindNode(d.self)
}

func (d *DHTNode) RoutingTableSize() int {
	return d.table.Count()
}

func (d *DHTNode) Close() {
	// no-op sekarang, disiapin buat cleanup goroutine kalau ada refresh
	// loop background nanti (bucket refresh periodik dsb)
}

// --- internal: iterative lookup ---

func (d *DHTNode) iterativeFindNode(target NodeID) []Contact {
	seen := make(map[NodeID]bool)
	shortlist := d.table.Closest(target, Alpha)

	for i := 0; i < 5 && len(shortlist) > 0; i++ {
		var next []Contact
		for _, c := range shortlist {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true

			contacts, err := d.queryFindNode(c, target)
			if err != nil {
				continue
			}
			for _, nc := range contacts {
				d.table.Update(nc)
				next = append(next, nc)
			}
		}
		if len(next) == 0 {
			break
		}
		shortlist = closestN(append(shortlist, next...), target, BucketK)
	}
	return shortlist
}

func (d *DHTNode) queryFindNode(contact Contact, target NodeID) ([]Contact, error) {
	client, err := d.dialOrGet(contact)
	if err != nil {
		return nil, err
	}

	req, _ := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeDHTFindNode, "", d.selfPeerIDString(), websocketpkg.DHTFindNodePayload{
		TargetID: target.String(),
	})
	resp, err := d.rpc.Call(client, req, websocketpkg.DefaultTimeout())
	if err != nil {
		return nil, err
	}

	var payload websocketpkg.DHTFindNodeRespPayload
	if err := parsePayload(resp.Payload, &payload); err != nil {
		return nil, err
	}
	return contactsFromWire(payload.Contacts), nil
}

func (d *DHTNode) queryFindValue(contact Contact, key string) ([]string, []Contact, error) {
	client, err := d.dialOrGet(contact)
	if err != nil {
		return nil, nil, err
	}

	req, _ := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeDHTFindValue, "", d.selfPeerIDString(), websocketpkg.DHTFindValuePayload{Key: key})
	resp, err := d.rpc.Call(client, req, websocketpkg.DefaultTimeout())
	if err != nil {
		return nil, nil, err
	}

	var payload websocketpkg.DHTFindValueRespPayload
	if err := parsePayload(resp.Payload, &payload); err != nil {
		return nil, nil, err
	}
	if payload.Found {
		return payload.Values, nil, nil
	}
	return nil, contactsFromWire(payload.Contacts), nil
}

func (d *DHTNode) sendStore(contact Contact, key, providerID string) {
	client, err := d.dialOrGet(contact)
	if err != nil {
		return
	}
	msg, _ := websocketpkg.NewMessage(websocketpkg.MsgTypeDHTStore, d.selfPeerIDString(), websocketpkg.DHTStorePayload{
		Key:     key,
		Value:   providerID,
		TTLSecs: 24 * 3600,
	})
	client.SendMessage(msg)
}

// dialOrGet nyari client yang udah konek di Hub berdasarkan contact ID.
// Kalau belum konek, kita gak punya URL dial-nya dari sini (itu tanggung
// jawab p2p.Connector) — jadi kalau gak ketemu, return error, si iterative
// lookup akan skip node itu.
func (d *DHTNode) dialOrGet(contact Contact) (*websocketpkg.Client, error) {
	if client, ok := d.hub.GetClient(contact.ID.String()); ok {
		return client, nil
	}
	return nil, fmt.Errorf("dht: no active connection to %s", contact.ID.String())
}

func (d *DHTNode) selfPeerIDString() string {
	return d.self.String()
}

// --- helpers ---

func contactsToWire(contacts []Contact) []websocketpkg.DHTContact {
	out := make([]websocketpkg.DHTContact, len(contacts))
	for i, c := range contacts {
		out[i] = websocketpkg.DHTContact{ID: c.ID.String(), Address: c.Address}
	}
	return out
}

func contactsFromWire(wire []websocketpkg.DHTContact) []Contact {
	out := make([]Contact, 0, len(wire))
	for _, w := range wire {
		id, ok := NodeIDFromHex(w.ID)
		if !ok {
			continue
		}
		out = append(out, Contact{ID: id, Address: w.Address})
	}
	return out
}

func closestN(contacts []Contact, target NodeID, n int) []Contact {
	seen := make(map[NodeID]bool)
	unique := make([]Contact, 0, len(contacts))
	for _, c := range contacts {
		if !seen[c.ID] {
			seen[c.ID] = true
			unique = append(unique, c)
		}
	}
	rt := NewRoutingTable(target) // reuse sorting logic aja lewat table kosong
	for _, c := range unique {
		rt.Update(c)
	}
	return rt.Closest(target, n)
}

func parsePayload(raw []byte, v interface{}) error {
	return websocketpkg.ParsePayload(raw, v)
}
