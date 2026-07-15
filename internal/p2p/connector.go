package p2p

import (
        "sort"
	"sync"
	"time"

	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

// Connector otomatis nge-dial semua peer yang dikenal (dari PeerManager
// atau tracker) yang belum konek, tanpa butuh command manual apapun dari
// user. Ini yang bikin FLOW berperilaku kayak torrent client: begitu ada
// peer baru ke-discover (lewat tracker/DHT/announce), Connector langsung
// coba nyambung sendiri di background.
type Connector struct {
	hub        *websocketpkg.Hub
	handler    *websocketpkg.Handler
	selfPeerID string
	selfPubKey string

	mu      sync.Mutex
	dialing map[string]chan struct{}

	// meshCheck, kalau di-set, dipanggil buat cek apakah suatu peer ID
	// satu organisasi (mesh) sama kita — dial ke peer satu mesh dapet
	// prioritas: interval reconnect lebih cepat, dan mereka dicoba
	// duluan waktu WatchPeerManager iterasi banyak peer sekaligus.
	meshCheck func(peerID string) bool
}

func NewConnector(hub *websocketpkg.Hub, handler *websocketpkg.Handler, selfPeerID, selfPubKey string) *Connector {
	return &Connector{
		hub:        hub,
		handler:    handler,
		selfPeerID: selfPeerID,
		selfPubKey: selfPubKey,
		dialing:    make(map[string]chan struct{}),
	}
}

// SetMeshAwareness wires in a function that tells the connector whether
// a given peer ID belongs to the same enterprise Mesh as this node.
// This is what makes MeshController actually affect behavior instead of
// just storing membership data — same-mesh peers get faster reconnect
// backoff and get dialed first when there are multiple pending targets.
func (c *Connector) SetMeshAwareness(fn func(peerID string) bool) {
	c.meshCheck = fn
}

// EnsureConnected mulai auto-dial ke address ini kalau belum ada usaha
// dial yang jalan. Idempotent — aman dipanggil berulang buat address
// yang sama (misal dari peer_announce yang keulang).
func (c *Connector) EnsureConnected(address string) {
	c.EnsureConnectedWithPeerID(address, "")
}

// EnsureConnectedWithPeerID is like EnsureConnected but also takes the
// target's peer ID (when known) so mesh priority can be applied. When
// peerID is empty (e.g. dialing a bare tracker URL where we don't know
// the remote identity yet), it behaves exactly like EnsureConnected.
func (c *Connector) EnsureConnectedWithPeerID(address, peerID string) {
	if address == "" {
		return
	}

	c.mu.Lock()
	if _, exists := c.dialing[address]; exists {
		c.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	c.dialing[address] = stopCh
	c.mu.Unlock()

	sameMesh := peerID != "" && c.meshCheck != nil && c.meshCheck(peerID)
	if sameMesh {
		utils.LogInfo("connector: starting priority auto-dial to %s (same mesh)", address)
	} else {
		utils.LogInfo("connector: starting auto-dial to %s", address)
	}

	go websocketpkg.AutoDialWithPriority(c.hub, c.handler, address, c.selfPeerID, c.selfPubKey, sameMesh, stopCh)
}

func (c *Connector) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for addr, stopCh := range c.dialing {
		close(stopCh)
		delete(c.dialing, addr)
	}
}

// WatchPeerManager polling PeerManager secara berkala dan auto-dial
// semua peer yang punya address tapi belum konek — dipakai buat peer
// yang di-discover lewat DHT/tracker, bukan cuma tracker utama.
func (c *Connector) WatchPeerManager(pm *PeerManager, interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			c.StopAll()
			return
		case <-ticker.C:
			peers := pm.List()

			// same-mesh peers first: if there's a lot of peers known and
			// only some goroutine-scheduling slack, this ordering biases
			// toward organizationally-important connections establishing
			// sooner rather than being interleaved arbitrarily.
			sortPeersByMeshPriority(peers, c.meshCheck)

			for _, p := range peers {
				if p.Address == "" || p.Locked {
					continue
				}
				addr := "ws://" + p.Address + ":" + itoa(p.Port) + "/ws"
				c.EnsureConnectedWithPeerID(addr, p.ID)
			}
		}
	}
}

func sortPeersByMeshPriority(peers []*Peer, meshCheck func(peerID string) bool) {
	if meshCheck == nil {
		return
	}
	sort.SliceStable(peers, func(i, j int) bool {
		iMesh := meshCheck(peers[i].ID)
		jMesh := meshCheck(peers[j].ID)
		return iMesh && !jMesh
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
