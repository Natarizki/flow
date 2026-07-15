package cache

import (
	"os"
	"path/filepath"

	"github.com/Natarizki/flow/pkg/utils"
)

type Storage struct {
	basePath string
}

func NewStorage(basePath string) (*Storage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, utils.WrapError("STORAGE_INIT", "failed to create cache dir", err)
	}
	return &Storage{basePath: basePath}, nil
}

func (s *Storage) pathFor(hash string) string {
	if len(hash) >= 4 {
		return filepath.Join(s.basePath, hash[:2], hash[2:4], hash)
	}
	return filepath.Join(s.basePath, hash)
}

func (s *Storage) Write(hash string, data []byte) error {
	path := s.pathFor(hash)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return utils.WrapError("STORAGE_WRITE", "failed to create shard dir", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return utils.WrapError("STORAGE_WRITE", "failed to write cache entry", err)
	}
	return nil
}

func (s *Storage) Read(hash string) ([]byte, error) {
	path := s.pathFor(hash)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, utils.ErrPeerNotFound
		}
		return nil, utils.WrapError("STORAGE_READ", "failed to read cache entry", err)
	}
	return data, nil
}

func (s *Storage) Exists(hash string) bool {
	path := s.pathFor(hash)
	_, err := os.Stat(path)
	return err == nil
}

func (s *Storage) Delete(hash string) error {
	path := s.pathFor(hash)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return utils.WrapError("STORAGE_DELETE", "failed to delete cache entry", err)
	}
	return nil
}

func (s *Storage) Size(hash string) (int64, error) {
	path := s.pathFor(hash)
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *Storage) TotalSize() (int64, error) {
	var total int64
	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func (s *Storage) BasePath() string {
	return s.basePath
}
