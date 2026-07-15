package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// GenerateSelfSignedCert bikin cert+key self-signed buat WSS server.
// P2P nature FLOW berarti gak ada CA terpusat — kepercayaan datang dari
// verifikasi Ed25519 fingerprint di layer aplikasi (lihat auth.go), bukan
// dari chain-of-trust certificate kayak web browser biasa. TLS di sini
// murni buat enkripsi transport, bukan buat identity verification.
func GenerateSelfSignedCert(certPath, keyPath string) error {
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return nil // udah ada, skip generate ulang
		}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return utils.WrapError("TLS_KEYGEN", "failed to generate ECDSA key", err)
	}

	serialNumber, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{Organization: []string{"FLOW P2P Node"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour), // 10 tahun, self-signed jadi gak butuh renewal ribet
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return utils.WrapError("TLS_CERTGEN", "failed to create certificate", err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return utils.WrapError("TLS_MKDIR", "failed to create cert dir", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return utils.WrapError("TLS_CERT_WRITE", "failed to create cert file", err)
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return utils.WrapError("TLS_KEY_MARSHAL", "failed to marshal private key", err)
	}
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return utils.WrapError("TLS_KEY_WRITE", "failed to create key file", err)
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	utils.LogInfo("generated self-signed TLS certificate at %s", certPath)
	return nil
}

// ServerTLSConfig balikin tls.Config yang locked ke TLS 1.3 doang —
// sesuai spec proyek ("TLS 1.3 Only" ada di daftar 53 fitur).
func ServerTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, utils.WrapError("TLS_LOAD", "failed to load cert/key pair", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig buat dial keluar ke peer lain. InsecureSkipVerify
// sengaja true di sini karena identitas peer diverifikasi di layer
// aplikasi (Ed25519 challenge-response), bukan lewat X.509 chain — sesuai
// arsitektur P2P tanpa CA terpusat. Verifikasi identitas HARUS tetap
// dilakukan lewat security.VerifyPeer setelah handshake, TLS di sini
// cuma ngasih enkripsi transport.
func ClientTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
}
