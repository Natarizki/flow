package fetcher

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Natarizki/flow/internal/cache"
	"github.com/Natarizki/flow/internal/compression"
	"github.com/Natarizki/flow/internal/p2p"
	"github.com/Natarizki/flow/internal/security"
	"github.com/Natarizki/flow/pkg/utils"
)

const (
	defaultTimeout  = 15 * time.Second
	maxResponseSize = 64 * 1024 * 1024
	userAgent       = "FLOW/0.1.0 (+https://github.com/Natarizki/flow)"
)

// Fetcher ambil konten dari URL, compress, blind (opsional), simpen ke
// cache, DAN split jadi chunk yang masuk ke ChunkStore — ini yang bikin
// konten yang baru di-fetch langsung tersedia buat di-pull peer lain
// lewat protokol chunk_request/chunk_response, bukan cuma nyangkut di
// disk lokal doang.
type Fetcher struct {
	client         *http.Client
	storage        cache.BlobStore
	index          cache.IndexStore
	chunkStore     *p2p.ChunkStore
	defaultLevel   compression.QuantLevel
	blindingActive bool

	// p2pLookup, kalau di-set, dipanggil dulu sebelum HTTP GET — kalau
	// berhasil nemu & narik semua chunk dari peer lain, request ini
	// gak pernah nyentuh network origin sama sekali. Optional (nil-safe)
	// biar Fetcher tetap bisa dipakai standalone tanpa P2P wiring
	// (misal di test, atau video preroll yang emang harus HTTP Range).
	p2pLookup func(url string, level compression.QuantLevel) (*FetchResult, bool)
}

func NewFetcher(storage cache.BlobStore, index cache.IndexStore, chunkStore *p2p.ChunkStore, defaultLevel compression.QuantLevel, blindingEnabled bool) *Fetcher {
	return &Fetcher{
		client:         &http.Client{Timeout: defaultTimeout},
		storage:        storage,
		index:          index,
		chunkStore:     chunkStore,
		defaultLevel:   defaultLevel,
		blindingActive: blindingEnabled,
	}
}

// SetP2PLookup wires in a function that attempts to satisfy a fetch
// entirely from the P2P network (DHT lookup + chunk pull) before
// falling back to a real HTTP request. This is what makes FLOW actually
// behave like a P2P cache instead of "fetch from internet, then
// incidentally also share it" — the network origin is now the last
// resort, not the first.
func (f *Fetcher) SetP2PLookup(fn func(url string, level compression.QuantLevel) (*FetchResult, bool)) {
	f.p2pLookup = fn
}

func (f *Fetcher) SetBlindingEnabled(enabled bool) {
	f.blindingActive = enabled
}

type FetchResult struct {
	URL            string
	Hash           string
	ContentType    string
	OriginalSize   int64
	CompressedSize int64
	Level          compression.QuantLevel
	FromCache      bool
	Blinded        bool
	ChunkHashes    []string // kosong kalau FromCache=true (chunk-nya udah ada dari fetch sebelumnya)
}

func (f *Fetcher) Fetch(url string) (*FetchResult, error) {
	return f.FetchWithLevel(url, f.defaultLevel)
}

func (f *Fetcher) FetchWithLevel(url string, level compression.QuantLevel) (*FetchResult, error) {
	cacheKey := utils.HashBytes([]byte(fmt.Sprintf("%s|%d", url, level)))

	if entry, ok := f.index.Get(cacheKey); ok {
		return &FetchResult{
			URL: url, Hash: cacheKey, ContentType: entry.ContentType,
			CompressedSize: entry.Size, Level: level, FromCache: true, Blinded: entry.Blinded,
		}, nil
	}

	// P2P-first: before touching the network origin at all, try to pull
	// this content from another peer that already has it (via DHT
	// provider lookup + chunk pull). Only fall through to HTTP if no
	// peer has it, or the pull fails/times out.
	if f.p2pLookup != nil {
		if result, ok := f.p2pLookup(url, level); ok {
			utils.LogInfo("fetched %s from P2P network (skipped origin HTTP request)", url)
			return result, nil
		}
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, utils.WrapError("FETCH_REQUEST", "failed to build request", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, utils.WrapError("FETCH_NETWORK", fmt.Sprintf("failed to fetch %s", url), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, utils.NewError("FETCH_HTTP_ERROR", fmt.Sprintf("%s returned status %d", url, resp.StatusCode))
	}

	limited := io.LimitReader(resp.Body, maxResponseSize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, utils.WrapError("FETCH_READ", "failed to read response body", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = guessContentTypeFromURL(url)
	} else {
		contentType = stripCharset(contentType)
	}

	flowFile, err := compression.Compress(data, level, url, contentType)
	if err != nil {
		return nil, utils.WrapError("FETCH_COMPRESS", "failed to compress fetched content", err)
	}

	encoded, err := compression.Encode(flowFile)
	if err != nil {
		return nil, utils.WrapError("FETCH_ENCODE", "failed to encode flow file", err)
	}

	toStore := encoded
	blinded := false
	if f.blindingActive {
		blindedBytes, err := security.Blind(encoded, url)
		if err != nil {
			return nil, utils.WrapError("FETCH_BLIND", "failed to blind content", err)
		}
		toStore = blindedBytes
		blinded = true
	}

	if err := f.storage.Write(cacheKey, toStore); err != nil {
		return nil, utils.WrapError("FETCH_STORE", "failed to write to storage", err)
	}

	f.index.Put(&cache.IndexEntry{
		Hash: cacheKey, URL: url, Size: int64(len(toStore)),
		ContentType: contentType, QuantLevel: int(level), Blinded: blinded,
	})

	// split jadi chunk dan masukin ke ChunkStore — bytes yang di-split
	// adalah TOSTORE (post-blind kalau aktif), sama persis kayak yang ada
	// di disk, jadi rekonstruksi dari chunk manapun bakal cocok dengan
	// file mentah di storage.
	var chunkHashes []string
	if f.chunkStore != nil {
		_, chunks := p2p.SplitIntoChunks(toStore, p2p.DefaultChunkSize)
		for _, chunk := range chunks {
			f.chunkStore.Add(cacheKey, chunk)
			chunkHashes = append(chunkHashes, chunk.Hash)
		}
	}

	utils.LogInfo("fetched %s -> %d bytes -> %d bytes stored in %d chunk(s) (level %d, blinded=%v)",
		url, len(data), len(toStore), len(chunkHashes), level, blinded)

	return &FetchResult{
		URL: url, Hash: cacheKey, ContentType: contentType,
		OriginalSize: int64(len(data)), CompressedSize: int64(len(toStore)),
		Level: level, FromCache: false, Blinded: blinded, ChunkHashes: chunkHashes,
	}, nil
}

// StoreRaw compresses and caches data that's already been retrieved by
// the caller (e.g. a video preroll byte-range fetch), skipping the HTTP
// GET entirely. This is what makes video preroll actually cheap: the
// preroller does ONE ranged request for 512KB, and this stores exactly
// that — it never re-fetches the full resource.
func (f *Fetcher) StoreRaw(url string, data []byte, contentType string, level compression.QuantLevel) (*FetchResult, error) {
	cacheKey := utils.HashBytes([]byte(fmt.Sprintf("%s|%d", url, level)))

	if entry, ok := f.index.Get(cacheKey); ok {
		return &FetchResult{
			URL: url, Hash: cacheKey, ContentType: entry.ContentType,
			CompressedSize: entry.Size, Level: level, FromCache: true, Blinded: entry.Blinded,
		}, nil
	}

	flowFile, err := compression.Compress(data, level, url, contentType)
	if err != nil {
		return nil, utils.WrapError("STORE_RAW_COMPRESS", "failed to compress raw content", err)
	}

	encoded, err := compression.Encode(flowFile)
	if err != nil {
		return nil, utils.WrapError("STORE_RAW_ENCODE", "failed to encode flow file", err)
	}

	toStore := encoded
	blinded := false
	if f.blindingActive {
		blindedBytes, err := security.Blind(encoded, url)
		if err != nil {
			return nil, utils.WrapError("STORE_RAW_BLIND", "failed to blind content", err)
		}
		toStore = blindedBytes
		blinded = true
	}

	if err := f.storage.Write(cacheKey, toStore); err != nil {
		return nil, utils.WrapError("STORE_RAW_WRITE", "failed to write to storage", err)
	}

	f.index.Put(&cache.IndexEntry{
		Hash: cacheKey, URL: url, Size: int64(len(toStore)),
		ContentType: contentType, QuantLevel: int(level), Blinded: blinded,
	})

	var chunkHashes []string
	if f.chunkStore != nil {
		_, chunks := p2p.SplitIntoChunks(toStore, p2p.DefaultChunkSize)
		for _, chunk := range chunks {
			f.chunkStore.Add(cacheKey, chunk)
			chunkHashes = append(chunkHashes, chunk.Hash)
		}
	}

	utils.LogInfo("stored raw content for %s -> %d bytes -> %d bytes in %d chunk(s) (level %d, blinded=%v)",
		url, len(data), len(toStore), len(chunkHashes), level, blinded)

	return &FetchResult{
		URL: url, Hash: cacheKey, ContentType: contentType,
		OriginalSize: int64(len(data)), CompressedSize: int64(len(toStore)),
		Level: level, FromCache: false, Blinded: blinded, ChunkHashes: chunkHashes,
	}, nil
}

func (f *Fetcher) ReadDecoded(hash string) ([]byte, error) {
	entry, ok := f.index.Get(hash)
	if !ok {
		return nil, utils.ErrPeerNotFound
	}

	raw, err := f.storage.Read(hash)
	if err != nil {
		return nil, err
	}

	if entry.Blinded {
		raw, err = security.Unblind(raw, entry.URL)
		if err != nil {
			return nil, utils.WrapError("FETCH_UNBLIND", "failed to unblind content (wrong source URL?)", err)
		}
	}

	flowFile, err := compression.Decode(raw)
	if err != nil {
		return nil, err
	}

	if entry.ContentType != "" && hasImagePrefix(entry.ContentType) && flowFile.Level != compression.LevelLossless {
		return flowFile.Data, nil
	}
	buf, err := compression.DecodeBuffer(flowFile.Data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hasImagePrefix(ct string) bool {
	return len(ct) >= 6 && ct[:6] == "image/"
}

func stripCharset(contentType string) string {
	for i, c := range contentType {
		if c == ';' {
			return contentType[:i]
		}
	}
	return contentType
}

func guessContentTypeFromURL(url string) string {
	switch {
	case hasSuffix(url, ".html"), hasSuffix(url, ".htm"), hasSuffix(url, "/"):
		return "text/html"
	case hasSuffix(url, ".png"):
		return "image/png"
	case hasSuffix(url, ".jpg"), hasSuffix(url, ".jpeg"):
		return "image/jpeg"
	case hasSuffix(url, ".css"):
		return "text/css"
	case hasSuffix(url, ".js"):
		return "application/javascript"
	default:
		return "text/html"
	}
}

func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
