package social

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type Badge struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tier        string `json:"tier"` // bronze, silver, gold, platinum
}

// BadgeCatalog adalah 22 badge yang unlock-nya dicek terhadap Stats
// beneran (bukan asal kasih semua badge pas register). Kategorinya:
// sharing (ngelayani konten), caching (nyimpen konten), networking
// (nemenin banyak peer), longevity (durasi aktif).
var BadgeCatalog = []Badge{
	{"first_share", "First Contact", "Served your first chunk to a peer", "bronze"},
	{"sharer_bronze", "Community Helper", "Served 100 chunks total", "bronze"},
	{"sharer_silver", "Bandwidth Buddy", "Served 1,000 chunks total", "silver"},
	{"sharer_gold", "Distribution Hub", "Served 10,000 chunks total", "gold"},
	{"sharer_platinum", "Backbone Node", "Served 100,000 chunks total", "platinum"},
	{"data_1mb", "Kilobyte Contributor", "Served 1 MB of data total", "bronze"},
	{"data_1gb", "Gigabyte Giver", "Served 1 GB of data total", "silver"},
	{"data_10gb", "Data Titan", "Served 10 GB of data total", "gold"},
	{"data_100gb", "Bandwidth Behemoth", "Served 100 GB of data total", "platinum"},
	{"cacher_bronze", "Pack Rat", "Cached 50 entries", "bronze"},
	{"cacher_silver", "Archive Keeper", "Cached 500 entries", "silver"},
	{"cacher_gold", "Vault Master", "Cached 5,000 entries", "gold"},
	{"first_peer", "Making Friends", "Connected to your first peer", "bronze"},
	{"networker_bronze", "Social Node", "Connected to 5 different peers", "bronze"},
	{"networker_silver", "Well Connected", "Connected to 25 different peers", "silver"},
	{"networker_gold", "Network Hub", "Connected to 100 different peers", "gold"},
	{"prefetch_user", "Fortune Teller", "Enabled predictive prefetch", "bronze"},
	{"prefetch_trained", "Pattern Learner", "Trained the Markov predictor at least once", "silver"},
	{"quantizer", "Compression Enthusiast", "Compressed a file at level 4", "bronze"},
	{"exporter", "Backup Champion", "Exported the cache at least once", "bronze"},
	{"top_10", "Rising Star", "Reached top 10 on the global leaderboard", "silver"},
	{"top_1", "Legend", "Reached #1 on the global leaderboard", "platinum"},
}

type UnlockedBadge struct {
	BadgeID    string    `json:"badge_id"`
	UnlockedAt time.Time `json:"unlocked_at"`
}

// Stats adalah snapshot metrik nyata yang dipakai buat cek unlock
// condition. Diisi dari data asli (Leaderboard, cache.Index, PeerManager,
// dll) — bukan angka yang bisa dipalsuin dari CLI langsung.
type Stats struct {
	ChunksServed      int64
	BytesServed       int64
	CacheEntries      int
	UniquePeersSeen   int
	PrefetchEnabled   bool
	PrefetchTrained   bool
	UsedLevel4Compress bool
	HasExported       bool
	LeaderboardRank   int
}

type AchievementManager struct {
	unlocked map[string]map[string]UnlockedBadge // peerID -> badgeID -> record
	mu       sync.RWMutex
	store    *store.Store
}

func NewAchievementManager(st *store.Store) *AchievementManager {
	a := &AchievementManager{
		unlocked: make(map[string]map[string]UnlockedBadge),
		store:    st,
	}
	if st != nil {
		a.loadFromStore()
	}
	return a
}

func (a *AchievementManager) loadFromStore() {
	entries, err := a.store.List("achievement:")
	if err != nil {
		utils.LogWarn("failed to load achievements from store: %v", err)
		return
	}
	for key, raw := range entries {
		// key format: "achievement:<peerID>:<badgeID>"
		var rec UnlockedBadge
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		parts := splitAchievementKey(key)
		if parts == nil {
			continue
		}
		peerID, badgeID := parts[0], parts[1]
		if a.unlocked[peerID] == nil {
			a.unlocked[peerID] = make(map[string]UnlockedBadge)
		}
		a.unlocked[peerID][badgeID] = rec
	}
	utils.LogInfo("loaded achievements for %d peers from persistent store", len(a.unlocked))
}

func splitAchievementKey(key string) []string {
	const prefix = "achievement:"
	if len(key) <= len(prefix) {
		return nil
	}
	rest := key[len(prefix):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			return []string{rest[:i], rest[i+1:]}
		}
	}
	return nil
}

// CheckAndUnlock evaluasi semua badge di catalog terhadap Stats yang
// dikasih, unlock yang belum ke-unlock dan memenuhi kondisi. Return
// slice badge yang BARU di-unlock kali ini (buat notifikasi).
func (a *AchievementManager) CheckAndUnlock(peerID string, s Stats) []Badge {
	var newlyUnlocked []Badge

	conditions := map[string]bool{
		"first_share":        s.ChunksServed >= 1,
		"sharer_bronze":      s.ChunksServed >= 100,
		"sharer_silver":      s.ChunksServed >= 1000,
		"sharer_gold":        s.ChunksServed >= 10000,
		"sharer_platinum":    s.ChunksServed >= 100000,
		"data_1mb":           s.BytesServed >= 1*1024*1024,
		"data_1gb":           s.BytesServed >= 1*1024*1024*1024,
		"data_10gb":          s.BytesServed >= 10*1024*1024*1024,
		"data_100gb":         s.BytesServed >= 100*1024*1024*1024,
		"cacher_bronze":      s.CacheEntries >= 50,
		"cacher_silver":      s.CacheEntries >= 500,
		"cacher_gold":        s.CacheEntries >= 5000,
		"first_peer":         s.UniquePeersSeen >= 1,
		"networker_bronze":   s.UniquePeersSeen >= 5,
		"networker_silver":   s.UniquePeersSeen >= 25,
		"networker_gold":     s.UniquePeersSeen >= 100,
		"prefetch_user":      s.PrefetchEnabled,
		"prefetch_trained":   s.PrefetchTrained,
		"quantizer":          s.UsedLevel4Compress,
		"exporter":           s.HasExported,
		"top_10":             s.LeaderboardRank > 0 && s.LeaderboardRank <= 10,
		"top_1":              s.LeaderboardRank == 1,
	}

	a.mu.Lock()
	if a.unlocked[peerID] == nil {
		a.unlocked[peerID] = make(map[string]UnlockedBadge)
	}
	for _, badge := range BadgeCatalog {
		if _, already := a.unlocked[peerID][badge.ID]; already {
			continue
		}
		if met, exists := conditions[badge.ID]; exists && met {
			rec := UnlockedBadge{BadgeID: badge.ID, UnlockedAt: time.Now()}
			a.unlocked[peerID][badge.ID] = rec
			newlyUnlocked = append(newlyUnlocked, badge)
			a.persistUnlock(peerID, rec)
		}
	}
	a.mu.Unlock()

	if len(newlyUnlocked) > 0 {
		utils.LogInfo("peer %s unlocked %d new achievement(s)", peerID, len(newlyUnlocked))
	}
	return newlyUnlocked
}

func (a *AchievementManager) persistUnlock(peerID string, rec UnlockedBadge) {
	if a.store == nil {
		return
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	a.store.Set("achievement:"+peerID+":"+rec.BadgeID, data)
}

func (a *AchievementManager) ListUnlocked(peerID string) []Badge {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []Badge
	unlockedSet := a.unlocked[peerID]
	for _, badge := range BadgeCatalog {
		if _, ok := unlockedSet[badge.ID]; ok {
			result = append(result, badge)
		}
	}
	return result
}

func (a *AchievementManager) HasBadge(peerID, badgeID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.unlocked[peerID][badgeID]
	return ok
}
