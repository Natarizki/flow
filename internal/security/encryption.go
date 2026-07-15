package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/Natarizki/flow/pkg/utils"
)

// Content blinding: konten yang disimpen di cache peer dienkripsi pakai
// key yang di-derive dari sesuatu yang cuma diketahui pihak yang minta
// konten itu (biasanya URL-nya sendiri) — mirip pola Freenet. Efeknya:
// peer yang nge-host cache gak bisa baca isi konten tanpa tau key
// derivation source-nya, walau file fisiknya ada di disk mereka.

// DeriveKey bikin AES-256 key dari sumber apapun (biasanya URL request).
func DeriveKey(source string) [32]byte {
	return sha256.Sum256([]byte(source))
}

// Blind enkripsi data pakai AES-256-GCM. Nonce random 12 byte diprepend
// ke ciphertext biar Unblind bisa extract balik.
func Blind(data []byte, source string) ([]byte, error) {
	key := DeriveKey(source)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, utils.WrapError("BLIND_CIPHER", "failed to create AES cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, utils.WrapError("BLIND_GCM", "failed to create GCM mode", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, utils.WrapError("BLIND_NONCE", "failed to generate nonce", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Unblind dekripsi hasil Blind. Butuh source yang sama persis buat
// re-derive key yang sama.
func Unblind(blinded []byte, source string) ([]byte, error) {
	key := DeriveKey(source)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, utils.WrapError("UNBLIND_CIPHER", "failed to create AES cipher", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, utils.WrapError("UNBLIND_GCM", "failed to create GCM mode", err)
	}

	nonceSize := gcm.NonceSize()
	if len(blinded) < nonceSize {
		return nil, fmt.Errorf("blinded data too short")
	}

	nonce, ciphertext := blinded[:nonceSize], blinded[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, utils.WrapError("UNBLIND_DECRYPT", "decryption failed (wrong source or corrupted data)", err)
	}
	return plaintext, nil
}
