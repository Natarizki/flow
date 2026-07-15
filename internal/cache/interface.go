package cache

// IndexStore is the minimal interface Fetcher needs — satisfied by both
// the persistent Index (BadgerDB-backed) and IncognitoIndex (in-memory
// only), so a single Fetcher codepath works for both normal and
// incognito sessions without duplicating fetch/compress/blind logic.
type IndexStore interface {
	Put(entry *IndexEntry)
	Get(hash string) (*IndexEntry, bool)
	Stats() (count int, totalSize int64)
}
type BlobStore interface {
	Write(hash string, data []byte) error
	Read(hash string) ([]byte, error)
	Delete(hash string) error
}
