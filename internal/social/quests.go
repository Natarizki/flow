package social

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type Quest struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Target      int64  `json:"target"`
	RewardScore int64  `json:"reward_score"`
}

// DailyQuestSet is what quests are active *today* — regenerated each
// UTC day from a rotating pool so it's not the same 3 quests forever.
var questPool = []Quest{
	{"serve_10_chunks", "Serve 10 chunks to other peers", 10, 50},
	{"serve_50_chunks", "Serve 50 chunks to other peers", 50, 200},
	{"fetch_5_pages", "Fetch and cache 5 new pages", 5, 40},
	{"connect_3_peers", "Connect to 3 different peers", 3, 60},
	{"compress_level3", "Compress a file at level 3 or higher", 1, 30},
	{"export_cache_once", "Export your cache once", 1, 20},
}

type QuestProgress struct {
	QuestID   string `json:"quest_id"`
	Progress  int64  `json:"progress"`
	Completed bool   `json:"completed"`
	ClaimedAt string `json:"claimed_at,omitempty"`
}

type dailyRecord struct {
	Date     string                    `json:"date"`
	QuestIDs []string                  `json:"quest_ids"`
	Progress map[string]*QuestProgress `json:"progress"` // peerID -> per-quest progress collapsed... see below
}

// QuestManager tracks per-peer progress against today's quest set,
// persisted so progress survives restarts within the same UTC day.
type QuestManager struct {
	mu       sync.Mutex
	store    *store.Store
	today    string
	questSet []Quest
	// progress[date][peerID][questID] = progress record
	progress map[string]map[string]map[string]*QuestProgress
}

func NewQuestManager(st *store.Store) *QuestManager {
	qm := &QuestManager{
		store:    st,
		progress: make(map[string]map[string]map[string]*QuestProgress),
	}
	qm.rollDailySet()
	if st != nil {
		qm.loadFromStore()
	}
	return qm
}

func (qm *QuestManager) rollDailySet() {
	date := time.Now().UTC().Format("2006-01-02")
	if qm.today == date {
		return
	}
	qm.today = date

	// deterministic pick based on date so all peers see the same quests
	// for the day, without needing coordination — hash the date string
	// to pick a rotating slice of the pool.
	seed := int(hashString(date))
	picked := make([]Quest, 0, 3)
	for i := 0; i < 3; i++ {
		picked = append(picked, questPool[(seed+i)%len(questPool)])
	}
	qm.questSet = picked
	utils.LogInfo("rolled new daily quest set for %s: %d quests", date, len(picked))
}

func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

func (qm *QuestManager) loadFromStore() {
	entries, err := qm.store.List("quest:")
	if err != nil {
		utils.LogWarn("failed to load quest progress: %v", err)
		return
	}
	for key, raw := range entries {
		// key format: "quest:<date>:<peerID>:<questID>"
		var rec QuestProgress
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		parts := splitQuestKey(key)
		if parts == nil {
			continue
		}
		date, peerID, questID := parts[0], parts[1], parts[2]
		if qm.progress[date] == nil {
			qm.progress[date] = make(map[string]map[string]*QuestProgress)
		}
		if qm.progress[date][peerID] == nil {
			qm.progress[date][peerID] = make(map[string]*QuestProgress)
		}
		qm.progress[date][peerID][questID] = &rec
	}
}

func splitQuestKey(key string) []string {
	const prefix = "quest:"
	if len(key) <= len(prefix) {
		return nil
	}
	rest := key[len(prefix):]
	var parts []string
	start := 0
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			parts = append(parts, rest[start:i])
			start = i + 1
		}
	}
	parts = append(parts, rest[start:])
	if len(parts) != 3 {
		return nil
	}
	return parts
}

// TodaysQuests returns the active quest set (rolling it if the UTC day
// has changed since last check).
func (qm *QuestManager) TodaysQuests() []Quest {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.rollDailySet()
	return qm.questSet
}

// RecordProgress increments a peer's progress on a named quest ID (if
// it's part of today's set) and marks it completed once target is hit.
// Returns the quest + true if this call just completed it.
func (qm *QuestManager) RecordProgress(peerID, questID string, amount int64) (Quest, bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.rollDailySet()

	var target Quest
	found := false
	for _, q := range qm.questSet {
		if q.ID == questID {
			target = q
			found = true
			break
		}
	}
	if !found {
		return Quest{}, false
	}

	if qm.progress[qm.today] == nil {
		qm.progress[qm.today] = make(map[string]map[string]*QuestProgress)
	}
	if qm.progress[qm.today][peerID] == nil {
		qm.progress[qm.today][peerID] = make(map[string]*QuestProgress)
	}
	rec, ok := qm.progress[qm.today][peerID][questID]
	if !ok {
		rec = &QuestProgress{QuestID: questID}
		qm.progress[qm.today][peerID][questID] = rec
	}
	if rec.Completed {
		return target, false // already done today
	}

	rec.Progress += amount
	justCompleted := false
	if rec.Progress >= target.Target {
		rec.Completed = true
		rec.ClaimedAt = time.Now().Format(time.RFC3339)
		justCompleted = true
	}

	qm.persist(qm.today, peerID, rec)
	return target, justCompleted
}

func (qm *QuestManager) persist(date, peerID string, rec *QuestProgress) {
	if qm.store == nil {
		return
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	qm.store.Set("quest:"+date+":"+peerID+":"+rec.QuestID, data)
}

func (qm *QuestManager) ProgressFor(peerID string) []QuestProgress {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.rollDailySet()

	var result []QuestProgress
	for _, q := range qm.questSet {
		if rec, ok := qm.progress[qm.today][peerID][q.ID]; ok {
			result = append(result, *rec)
		} else {
			result = append(result, QuestProgress{QuestID: q.ID})
		}
	}
	return result
}
