package cache

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

type IndexEntry struct {
	Hash        string    `json:"hash"`
	URL         string    `json:"url,omitempty"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type,omitempty"`
	QuantLevel  int       `json:"quant_level"`
	Blinded     bool      `json:"blinded"`
	CreatedAt   time.Time `json:"created_at"`
	LastAccess  time.Time `json:"last_access"`
	AccessCount int64     `json:"access_count"`
}

type Index struct {
	entries map[string]*IndexEntry
	mu      sync.RWMutex
	lru     *LRU
	storage *Storage
}

func NewIndex(maxSize int64, storage *Storage) *Index {
	idx := &Index{
		entries: make(map[string]*IndexEntry),
		lru:     NewLRU(maxSize),
		storage: storage,
	}
	idx.lru.OnEvict(func(hash string) {
		idx.mu.Lock()
		delete(idx.entries, hash)
		idx.mu.Unlock()
		storage.Delete(hash)
	})
	return idx
}

func (idx *Index) Put(entry *IndexEntry) {
	idx.mu.Lock()
	entry.CreatedAt = time.Now()
	entry.LastAccess = time.Now()
	idx.entries[entry.Hash] = entry
	idx.mu.Unlock()

	idx.lru.Touch(entry.Hash, entry.Size)
}

func (idx *Index) Get(hash string) (*IndexEntry, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entry, ok := idx.entries[hash]
	if ok {
		entry.LastAccess = time.Now()
		entry.AccessCount++
		idx.lru.Touch(hash, entry.Size)
	}
	return entry, ok
}

func (idx *Index) Remove(hash string) {
	idx.mu.Lock()
	delete(idx.entries, hash)
	idx.mu.Unlock()
	idx.lru.Remove(hash)
}

func (idx *Index) List() []*IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]*IndexEntry, 0, len(idx.entries))
	for _, e := range idx.entries {
		result = append(result, e)
	}
	return result
}

func (idx *Index) CleanOlderThan(d time.Duration) int {
	stale := idx.lru.StaleEntries(d)
	for _, hash := range stale {
		idx.Remove(hash)
		idx.storage.Delete(hash)
	}
	return len(stale)
}

func (idx *Index) Stats() (count int, totalSize int64) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	count = len(idx.entries)
	totalSize = idx.lru.CurrentSize()
	return
}

func (idx *Index) SetMaxSize(size int64) {
	idx.lru.SetMaxSize(size)
}
// ExportArchive nulis tar archive berisi:
//   - "index.json"  -> snapshot semua IndexEntry
//   - "blobs/<hash>" -> isi file cache mentah per entry
func (idx *Index) ExportArchive(destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return utils.WrapError("CACHE_EXPORT", "failed to create archive", err)
	}
	defer out.Close()

	tw := tar.NewWriter(out)
	defer tw.Close()

	entries := idx.List()
	indexJSON, err := json.Marshal(entries)
	if err != nil {
		return utils.WrapError("CACHE_EXPORT", "failed to marshal index", err)
	}

	if err := writeTarEntry(tw, "index.json", indexJSON); err != nil {
		return err
	}

	for _, e := range entries {
		data, err := idx.storage.Read(e.Hash)
		if err != nil {
			utils.LogWarn("skip missing blob %s during export: %v", e.Hash, err)
			continue
		}
		if err := writeTarEntry(tw, "blobs/"+e.Hash, data); err != nil {
			return err
		}
	}

	utils.LogInfo("exported %d cache entries to %s", len(entries), destPath)
	return nil
}

// ImportArchive baca tar archive hasil ExportArchive, tulis ulang tiap
// blob ke storage, dan restore metadata index entry.
func (idx *Index) ImportArchive(srcPath string) (int, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return 0, utils.WrapError("CACHE_IMPORT", "failed to open archive", err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	var indexEntries []*IndexEntry
	blobs := make(map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, utils.WrapError("CACHE_IMPORT", "failed to read archive", err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return 0, utils.WrapError("CACHE_IMPORT", "failed to read entry", err)
		}

		switch {
		case hdr.Name == "index.json":
			if err := json.Unmarshal(data, &indexEntries); err != nil {
				return 0, utils.WrapError("CACHE_IMPORT", "failed to parse index.json", err)
			}
		case len(hdr.Name) > 6 && hdr.Name[:6] == "blobs/":
			blobs[hdr.Name[6:]] = data
		}
	}

	imported := 0
	for _, e := range indexEntries {
		idx.mu.RLock()
		_, exists := idx.entries[e.Hash]
		idx.mu.RUnlock()
		if exists {
			continue
		}

		data, ok := blobs[e.Hash]
		if !ok {
			utils.LogWarn("import: blob missing for entry %s, skipping", e.Hash)
			continue
		}
		if err := idx.storage.Write(e.Hash, data); err != nil {
			utils.LogWarn("import: failed to write blob %s: %v", e.Hash, err)
			continue
		}
		idx.Put(e)
		imported++
	}

	utils.LogInfo("imported %d cache entries from %s", imported, srcPath)
	return imported, nil
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return utils.WrapError("CACHE_EXPORT", fmt.Sprintf("failed to write header for %s", name), err)
	}
	if _, err := tw.Write(data); err != nil {
		return utils.WrapError("CACHE_EXPORT", fmt.Sprintf("failed to write data for %s", name), err)
	}
	return nil
}

func (idx *Index) MarshalJSON() ([]byte, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return json.Marshal(idx.entries)
}
