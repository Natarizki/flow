package store

import (
	"github.com/dgraph-io/badger/v4"

	"github.com/Natarizki/flow/pkg/utils"
)

// Store adalah wrapper tipis di atas BadgerDB, dipakai buat persist data
// yang sebelumnya cuma hidup di in-memory map (users, sessions, peers).
// Restart daemon sekarang gak lagi ngilangin semua akun/peer.
type Store struct {
	db *badger.DB
}

func Open(path string) (*Store, error) {
	opts := badger.DefaultOptions(path).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, utils.WrapError("STORE_OPEN", "failed to open badger db at "+path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Set(key string, value []byte) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
	if err != nil {
		return utils.WrapError("STORE_SET", "failed to set key "+key, err)
	}
	return nil
}

func (s *Store) Get(key string) ([]byte, error) {
	var out []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			out = append([]byte{}, val...)
			return nil
		})
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, utils.ErrPeerNotFound
		}
		return nil, utils.WrapError("STORE_GET", "failed to get key "+key, err)
	}
	return out, nil
}

func (s *Store) Delete(key string) error {
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return utils.WrapError("STORE_DELETE", "failed to delete key "+key, err)
	}
	return nil
}

// List balikin semua key->value yang key-nya diawali prefix tertentu.
// Dipakai buat load semua "user:*", "session:*", "peer:*" pas startup.
func (s *Store) List(prefix string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		p := []byte(prefix)
		for it.Seek(p); it.ValidForPrefix(p); it.Next() {
			item := it.Item()
			key := string(item.KeyCopy(nil))
			err := item.Value(func(val []byte) error {
				result[key] = append([]byte{}, val...)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, utils.WrapError("STORE_LIST", "failed to list prefix "+prefix, err)
	}
	return result, nil
}
