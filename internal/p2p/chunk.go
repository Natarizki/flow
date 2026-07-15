package p2p

import (
	"fmt"
	"sync"

	"github.com/Natarizki/flow/pkg/utils"
)

const DefaultChunkSize = 256 * 1024 // 256KB per chunk

type Chunk struct {
	Hash   string
	Data   []byte
	Offset int64
	Length int64
}

type ChunkManifest struct {
	FileHash   string   `json:"file_hash"`
	TotalSize  int64    `json:"total_size"`
	ChunkSize  int64    `json:"chunk_size"`
	ChunkHashes []string `json:"chunk_hashes"`
}

func SplitIntoChunks(data []byte, chunkSize int64) (*ChunkManifest, []*Chunk) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	var chunks []*Chunk
	var hashes []string
	total := int64(len(data))

	for offset := int64(0); offset < total; offset += chunkSize {
		end := offset + chunkSize
		if end > total {
			end = total
		}
		chunkData := data[offset:end]
		hash := utils.HashBytes(chunkData)

		chunks = append(chunks, &Chunk{
			Hash:   hash,
			Data:   chunkData,
			Offset: offset,
			Length: end - offset,
		})
		hashes = append(hashes, hash)
	}

	fileHash := utils.HashBytes(data)

	manifest := &ChunkManifest{
		FileHash:    fileHash,
		TotalSize:   total,
		ChunkSize:   chunkSize,
		ChunkHashes: hashes,
	}

	return manifest, chunks
}

// ChunkStore melacak chunk mana yang tersedia secara lokal per file
type ChunkStore struct {
	available map[string]map[string]*Chunk // fileHash -> chunkHash -> Chunk
	mu        sync.RWMutex
}

func NewChunkStore() *ChunkStore {
	return &ChunkStore{
		available: make(map[string]map[string]*Chunk),
	}
}

func (cs *ChunkStore) Add(fileHash string, chunk *Chunk) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, ok := cs.available[fileHash]; !ok {
		cs.available[fileHash] = make(map[string]*Chunk)
	}
	cs.available[fileHash][chunk.Hash] = chunk
}

func (cs *ChunkStore) Get(fileHash, chunkHash string) (*Chunk, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	fileChunks, ok := cs.available[fileHash]
	if !ok {
		return nil, fmt.Errorf("no chunks known for file %s", utils.ShortHash(fileHash, 8))
	}
	chunk, ok := fileChunks[chunkHash]
	if !ok {
		return nil, fmt.Errorf("chunk %s not available", utils.ShortHash(chunkHash, 8))
	}
	return chunk, nil
}

func (cs *ChunkStore) HaveChunks(fileHash string) []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	fileChunks, ok := cs.available[fileHash]
	if !ok {
		return nil
	}
	hashes := make([]string, 0, len(fileChunks))
	for h := range fileChunks {
		hashes = append(hashes, h)
	}
	return hashes
}

func (cs *ChunkStore) HasFile(fileHash string, manifest *ChunkManifest) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	fileChunks, ok := cs.available[fileHash]
	if !ok {
		return false
	}
	for _, h := range manifest.ChunkHashes {
		if _, exists := fileChunks[h]; !exists {
			return false
		}
	}
	return true
}
