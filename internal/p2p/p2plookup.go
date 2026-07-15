package p2p

import (
	"sort"
	"time"

	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

type P2PFetchResult struct {
	Hash           string
	ContentType    string
	CompressedSize int64
	ChunkHashes    []string
}

func sortProvidersByReputation(providers []string, peers *PeerManager) []string {
	type scored struct {
		id    string
		score float64
	}
	list := make([]scored, 0, len(providers))
	for _, id := range providers {
		score := 0.0
		if p, ok := peers.Get(id); ok {
			score = p.Reputation
		}
		list = append(list, scored{id: id, score: score})
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].score > list[j].score
	})
	ordered := make([]string, len(list))
	for i, s := range list {
		ordered[i] = s.id
	}
	return ordered
}

// requestManifest actively asks a specific provider what chunks it has
// for a given content hash, rather than relying on having previously
// overheard a chunk_have gossip broadcast about it. This is the piece
// that makes P2P-first fetch work even for content this node has never
// heard about before — it can now ask.
func requestManifest(hub *websocketpkg.Hub, providerID, fileHash string, timeout time.Duration) ([]string, bool) {
	client, ok := hub.GetClient(providerID)
	if !ok {
		return nil, false
	}

	req, err := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeManifestRequest, "", "", websocketpkg.ManifestRequestPayload{FileHash: fileHash})
	if err != nil {
		return nil, false
	}

	resp, err := hub.RPC().Call(client, req, timeout)
	if err != nil {
		return nil, false
	}

	var payload websocketpkg.ManifestResponsePayload
	if err := websocketpkg.ParsePayload(resp.Payload, &payload); err != nil {
		return nil, false
	}
	if !payload.Found || len(payload.ChunkHashes) == 0 {
		return nil, false
	}
	return payload.ChunkHashes, true
}

// TryFetchFromNetwork is the real "check P2P before origin" path:
//  1. Ask DHT who has providers for this content hash.
//  2. If we don't already know the chunk manifest locally (from prior
//     gossip), actively ask each provider for it via requestManifest —
//     this is what lets a node fetch content it's never heard about.
//  3. Pull every chunk from providers, trying highest-reputation first,
//     verifying checksums via RequestChunk (which also updates peer
//     reputation).
//
// Returns ok=false if no provider is found, no provider will give us a
// manifest, or reassembly fails for any reason — callers fall back to a
// normal HTTP fetch.
func TryFetchFromNetwork(
	hub *websocketpkg.Hub,
	peers *PeerManager,
	dht *DHTNode,
	chunkStore *ChunkStore,
	storage BlobStoreWriter,
	selfPeerID, cacheKey string,
) (*P2PFetchResult, bool) {
	providers, err := dht.FindProviders(cacheKey)
	if err != nil || len(providers) == 0 {
		return nil, false
	}
	providers = sortProvidersByReputation(providers, peers)

	localChunks := chunkStore.HaveChunks(cacheKey)
	if len(localChunks) == 0 {
		// don't know the manifest yet — actively ask each provider
		// (highest reputation first) until one answers.
		for _, providerID := range providers {
			if providerID == selfPeerID {
				continue
			}
			if hashes, ok := requestManifest(hub, providerID, cacheKey, defaultChunkPullTimeout); ok {
				localChunks = hashes
				utils.LogInfo("p2p fetch: obtained manifest for %s from provider %s (%d chunks)",
					utils.ShortHash(cacheKey, 8), providerID, len(hashes))
				break
			}
		}
		if len(localChunks) == 0 {
			return nil, false // no provider would give us a manifest, give up on P2P
		}
	}

	var assembled []byte
	pulledFromAnyPeer := false

	for _, chunkHash := range localChunks {
		if chunk, err := chunkStore.Get(cacheKey, chunkHash); err == nil {
			assembled = append(assembled, chunk.Data...)
			continue
		}

		got := false
		for _, providerID := range providers {
			if providerID == selfPeerID {
				continue
			}
			data, err := RequestChunk(hub, peers, selfPeerID, providerID, cacheKey, chunkHash, defaultChunkPullTimeout)
			if err != nil {
				utils.LogWarn("p2p fetch: chunk pull from %s failed: %v", providerID, err)
				continue
			}
			assembled = append(assembled, data...)
			pulledFromAnyPeer = true
			got = true
			break
		}
		if !got {
			return nil, false
		}
	}

	if !pulledFromAnyPeer || len(assembled) == 0 {
		return nil, false
	}

	if err := storage.Write(cacheKey, assembled); err != nil {
		return nil, false
	}

	utils.LogInfo("p2p fetch: reassembled %d bytes from %d chunk(s) via network peers", len(assembled), len(localChunks))

	return &P2PFetchResult{
		Hash:           cacheKey,
		CompressedSize: int64(len(assembled)),
		ChunkHashes:    localChunks,
	}, true
}

type BlobStoreWriter interface {
	Write(hash string, data []byte) error
}

const defaultChunkPullTimeout = 8 * time.Second
