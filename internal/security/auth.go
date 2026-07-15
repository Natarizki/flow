package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/Natarizki/flow/pkg/utils"
)

// Identity adalah keypair Ed25519 permanen buat 1 node — beda dari JWT
// auth (yang buat user login ke dashboard), ini buat verifikasi identitas
// antar-peer di layer P2P: "peer yang connect ini emang beneran punya
// private key yang cocok sama fingerprint yang dia klaim".
type Identity struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// LoadOrCreateIdentity baca keypair dari disk kalau udah ada, atau
// generate baru dan simpen kalau belum. Dipanggil sekali pas daemon
// startup.
func LoadOrCreateIdentity(path string) (*Identity, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(data)
		return &Identity{
			PublicKey:  priv.Public().(ed25519.PublicKey),
			PrivateKey: priv,
		}, nil
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, utils.WrapError("IDENTITY_GEN", "failed to generate ed25519 keypair", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, utils.WrapError("IDENTITY_SAVE", "failed to create identity dir", err)
	}
	if err := os.WriteFile(path, priv, 0600); err != nil {
		return nil, utils.WrapError("IDENTITY_SAVE", "failed to write identity file", err)
	}

	utils.LogInfo("generated new peer identity, fingerprint: %s", Fingerprint(priv.Public().(ed25519.PublicKey)))

	return &Identity{
		PublicKey:  priv.Public().(ed25519.PublicKey),
		PrivateKey: priv,
	}, nil
}

// Fingerprint adalah representasi publik dari identitas node — ini yang
// dipakai sebagai bagian dari NodeID di Kademlia dan yang diverifikasi
// pas handshake.
func Fingerprint(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

func (id *Identity) FingerprintHex() string {
	return Fingerprint(id.PublicKey)
}

// Sign tandatangan data pakai private key node ini.
func (id *Identity) Sign(data []byte) []byte {
	return ed25519.Sign(id.PrivateKey, data)
}

// VerifyPeer cek signature yang diklaim dari peer dengan public key hex
// yang dia kasih. Return false kalau signature invalid ATAU public key
// hex-nya corrupt.
func VerifyPeer(pubKeyHex string, data, signature []byte) bool {
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pubBytes), data, signature)
}

// ChallengeResponse adalah protokol sederhana: node A kirim nonce random
// ke node B, B sign pakai private key-nya, A verify pakai public key
// yang B klaim di handshake. Kalau valid, A tau B beneran pegang private
// key yang cocok — bukan cuma ngaku-ngaku PeerID orang lain.
type Challenge struct {
	Nonce []byte
}

func NewChallenge() *Challenge {
	nonce := make([]byte, 32)
	rand.Read(nonce)
	return &Challenge{Nonce: nonce}
}

func (ch *Challenge) VerifyResponse(pubKeyHex string, signature []byte) bool {
	return VerifyPeer(pubKeyHex, ch.Nonce, signature)
}
