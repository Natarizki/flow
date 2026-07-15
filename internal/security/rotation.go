package security

import (
	"crypto/tls"
	"os"
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// CertRotator periodically regenerates the self-signed TLS cert/key pair
// and hot-swaps the active tls.Config's certificate without dropping the
// listener — uses tls.Config.GetCertificate so live connections keep
// their already-negotiated cert, and new connections pick up the fresh
// one automatically.
type CertRotator struct {
	certPath string
	keyPath  string
	interval time.Duration

	mu   sync.RWMutex
	cert *tls.Certificate
}

func NewCertRotator(certPath, keyPath string, interval time.Duration) (*CertRotator, error) {
	r := &CertRotator{certPath: certPath, keyPath: keyPath, interval: interval}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *CertRotator) reload() error {
	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		return utils.WrapError("CERT_ROTATE_LOAD", "failed to load cert for rotation", err)
	}
	r.mu.Lock()
	r.cert = &cert
	r.mu.Unlock()
	return nil
}

// GetCertificate satisfies tls.Config.GetCertificate — the TLS stack
// calls this on every new handshake, so it always gets whatever cert is
// currently active, even mid-run after a rotation.
func (r *CertRotator) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert, nil
}

// Run regenerates the cert on disk every `interval` and reloads it into
// memory — actual rotation, not just a timer that logs "would rotate".
func (r *CertRotator) Run(stopCh <-chan struct{}) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if err := regenerateCert(r.certPath, r.keyPath); err != nil {
				utils.LogWarn("certificate rotation failed: %v", err)
				continue
			}
			if err := r.reload(); err != nil {
				utils.LogWarn("certificate reload after rotation failed: %v", err)
				continue
			}
			utils.LogInfo("TLS certificate rotated successfully")
		}
	}
}

// regenerateCert forces a fresh self-signed cert regardless of whether
// one already exists (GenerateSelfSignedCert normally skips if files
// exist — rotation needs to force it).
func regenerateCert(certPath, keyPath string) error {
	if err := removeIfExists(certPath); err != nil {
		return err
	}
	if err := removeIfExists(keyPath); err != nil {
		return err
	}
	return GenerateSelfSignedCert(certPath, keyPath)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
