package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

func HashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ShortHash(fullHash string, length int) string {
	if len(fullHash) < length {
		return fullHash
	}
	return fullHash[:length]
}
