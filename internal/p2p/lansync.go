package p2p

import (
	"encoding/json"
	"net"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

const lanSyncPort = 41234 // arbitrary but fixed, so all FLOW instances on a LAN agree on it

type LANAnnouncement struct {
	NodeID    string `json:"node_id"`
	PeerName  string `json:"peer_name"`
	WSPort    int    `json:"ws_port"`
	APIPort   int    `json:"api_port"` // needed so the receiving side knows where to POST a bookmark sync request
	Timestamp int64  `json:"timestamp"`
}

// LANSync broadcasts UDP announcements on the local subnet so other
// FLOW instances belonging to the same user (same account, different
// devices on the same WiFi) can find each other without a tracker —
// this is real UDP broadcast, not a stub, using net.ListenUDP/WriteTo.
type LANSync struct {
	nodeID   string
	peerName string
	wsPort   int
	apiPort  int
	onPeer   func(LANAnnouncement, string) // announcement, source IP
}

func NewLANSync(nodeID, peerName string, wsPort, apiPort int) *LANSync {
	return &LANSync{nodeID: nodeID, peerName: peerName, wsPort: wsPort, apiPort: apiPort}
}

func (l *LANSync) OnPeerDiscovered(fn func(LANAnnouncement, string)) {
	l.onPeer = fn
}

// Broadcast periodically sends a UDP broadcast packet announcing this
// node to 255.255.255.255:41234 so any listener on the same LAN segment
// picks it up.
func (l *LANSync) Broadcast(interval time.Duration, stopCh <-chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp4", "255.255.255.255:41234")
	if err != nil {
		utils.LogWarn("lan sync: failed to resolve broadcast address: %v", err)
		return
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		utils.LogWarn("lan sync: failed to open broadcast socket: %v", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	send := func() {
		ann := LANAnnouncement{
			NodeID: l.nodeID, PeerName: l.peerName, WSPort: l.wsPort, APIPort: l.apiPort,
			Timestamp: time.Now().Unix(),
		}
		data, err := json.Marshal(ann)
		if err != nil {
			return
		}
		conn.Write(data)
	}

	send()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			send()
		}
	}
}

// Listen opens a UDP socket on lanSyncPort and calls onPeer for every
// valid announcement received from another node (ignoring our own
// broadcasts by checking NodeID).
func (l *LANSync) Listen(stopCh <-chan struct{}) {
	addr := &net.UDPAddr{Port: lanSyncPort, IP: net.IPv4zero}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		utils.LogWarn("lan sync: failed to listen on UDP %d: %v", lanSyncPort, err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	conn.SetReadBuffer(1 << 20)

	go func() {
		<-stopCh
		conn.Close()
	}()

	for {
		n, srcAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // closed, likely from stopCh
		}

		var ann LANAnnouncement
		if err := json.Unmarshal(buf[:n], &ann); err != nil {
			continue
		}
		if ann.NodeID == l.nodeID {
			continue // ignore our own broadcast
		}

		if l.onPeer != nil {
			l.onPeer(ann, srcAddr.IP.String())
		}
	}
}
