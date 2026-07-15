package enterprise

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// License validation offline (gak perlu phone-home ke server pusat) —
// license key adalah payload JSON yang di-sign HMAC-SHA256 pakai secret
// yang cuma dipegang penerbit license (Anthropic/vendor FLOW, bukan end
// user). Mirip struktur JWT tapi custom biar independen dari auth JWT
// user login.
type LicenseTier string

const (
	TierFree       LicenseTier = "free"
	TierEnterprise LicenseTier = "enterprise"
)

type LicensePayload struct {
	OrgName   string      `json:"org_name"`
	Tier      LicenseTier `json:"tier"`
	MaxPeers  int         `json:"max_peers"` // 0 = unlimited
	IssuedAt  time.Time   `json:"issued_at"`
	ExpiresAt time.Time   `json:"expires_at"`
}

var licenseSecret = []byte("flow-license-secret-change-in-production")

func SetLicenseSecret(secret string) {
	licenseSecret = []byte(secret)
}

// GenerateLicenseKey bikin license key string format "<base64 payload>.<base64 signature>"
// dipanggil dari sisi vendor/admin buat nerbitin license baru, bukan
// dari end-user daemon.
func GenerateLicenseKey(payload LicensePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", utils.WrapError("LICENSE_MARSHAL", "failed to marshal license payload", err)
	}

	sig := hmac.New(sha256.New, licenseSecret)
	sig.Write(data)
	signature := sig.Sum(nil)

	encodedPayload := base64.URLEncoding.EncodeToString(data)
	encodedSig := base64.URLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSig, nil
}

// ValidateLicenseKey verify signature + expiry. Kalau valid, balikin
// payload-nya. Ini yang dipanggil daemon startup buat aktifin fitur
// enterprise (mesh controller, advanced analytics, dll).
func ValidateLicenseKey(key string) (*LicensePayload, error) {
	var payloadB64, sigB64 string
	dotIdx := -1
	for i, c := range key {
		if c == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx == -1 {
		return nil, utils.NewError("LICENSE_MALFORMED", "license key missing signature separator")
	}
	payloadB64 = key[:dotIdx]
	sigB64 = key[dotIdx+1:]

	data, err := base64.URLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, utils.WrapError("LICENSE_DECODE", "failed to decode license payload", err)
	}
	sig, err := base64.URLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, utils.WrapError("LICENSE_DECODE", "failed to decode license signature", err)
	}

	expectedSig := hmac.New(sha256.New, licenseSecret)
	expectedSig.Write(data)
	if !hmac.Equal(sig, expectedSig.Sum(nil)) {
		return nil, utils.NewError("LICENSE_INVALID", "license signature does not match")
	}

	var payload LicensePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, utils.WrapError("LICENSE_PARSE", "failed to parse license payload", err)
	}

	if time.Now().After(payload.ExpiresAt) {
		return nil, utils.NewError("LICENSE_EXPIRED", fmt.Sprintf("license expired on %s", payload.ExpiresAt.Format(time.RFC3339)))
	}

	return &payload, nil
}

// LicenseManager nyimpen state license aktif buat 1 daemon instance.
type LicenseManager struct {
	active *LicensePayload
}

func NewLicenseManager() *LicenseManager {
	return &LicenseManager{}
}

func (lm *LicenseManager) Activate(key string) error {
	payload, err := ValidateLicenseKey(key)
	if err != nil {
		return err
	}
	lm.active = payload
	utils.LogInfo("license activated: org=%s tier=%s expires=%s", payload.OrgName, payload.Tier, payload.ExpiresAt.Format("2006-01-02"))
	return nil
}

func (lm *LicenseManager) IsEnterprise() bool {
	return lm.active != nil && lm.active.Tier == TierEnterprise
}

func (lm *LicenseManager) Current() *LicensePayload {
	return lm.active
}

func (lm *LicenseManager) MaxPeersAllowed() int {
	if lm.active == nil {
		return 0
	}
	return lm.active.MaxPeers
}
