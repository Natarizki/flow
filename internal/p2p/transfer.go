package p2p

import (
	"fmt"
	"time"

	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

// RequestChunk minta 1 chunk dari peer tertentu lewat RPC (chunk_request
// -> chunk_response, dikorelasikan pakai message ID lewat Hub.RPC()).
// Verifikasi checksum data yang balik, dan update reputation peer itu di
// PeerManager berdasarkan hasilnya — ini bikin field Reputation yang
// sebelumnya cuma ada di struct tapi gak pernah keupdate, sekarang
// beneran reflect perilaku nyata peer (kasih data valid = naik, timeout/
// corrupt = turun).
func RequestChunk(hub *websocketpkg.Hub, peers *PeerManager, selfPeerID, targetPeerID, fileHash, chunkHash string, timeout time.Duration) ([]byte, error) {
	client, ok := hub.GetClient(targetPeerID)
	if !ok {
		return nil, fmt.Errorf("no active connection to peer %s", targetPeerID)
	}

	reqPayload := websocketpkg.ChunkRequestPayload{
		FileHash:  fileHash,
		ChunkHash: chunkHash,
	}
	msg, err := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeChunkRequest, "", selfPeerID, reqPayload)
	if err != nil {
		return nil, err
	}

	resp, err := hub.RPC().Call(client, msg, timeout)
	if err != nil {
		peers.UpdateReputation(targetPeerID, -2.0)
		return nil, fmt.Errorf("chunk request to %s timed out: %w", targetPeerID, err)
	}

	var payload websocketpkg.ChunkResponsePayload
	if err := websocketpkg.ParsePayload(resp.Payload, &payload); err != nil {
		peers.UpdateReputation(targetPeerID, -1.0)
		return nil, fmt.Errorf("malformed chunk response from %s: %w", targetPeerID, err)
	}

	actualChecksum := utils.HashBytes(payload.Data)
	if actualChecksum != payload.Checksum || actualChecksum != chunkHash {
		peers.UpdateReputation(targetPeerID, -5.0) // data korup/gak match, penalti besar
		return nil, fmt.Errorf("checksum mismatch from peer %s (possible corruption)", targetPeerID)
	}

	peers.UpdateReputation(targetPeerID, 1.0) // data valid diterima, reward
	return payload.Data, nil
}
