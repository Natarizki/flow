package p2p

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/Natarizki/flow/internal/store"
	"github.com/Natarizki/flow/pkg/utils"
)

type Organization struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"invite_code"`
	MemberIDs  []string  `json:"member_ids"`
	CreatedAt  time.Time `json:"created_at"`
}

// OrgManager is the CLI/tracker-level counterpart of enterprise.Mesh —
// orgs are lighter-weight and any user can create one via `flc org
// create`, whereas Mesh is admin-provisioned and gated by enterprise
// license for routing priority. An org can later be linked to a Mesh
// once upgraded.
type OrgManager struct {
	orgs  map[string]*Organization
	mu    sync.RWMutex
	store *store.Store
}

func NewOrgManager(st *store.Store) *OrgManager {
	o := &OrgManager{orgs: make(map[string]*Organization), store: st}
	if st != nil {
		o.loadFromStore()
	}
	return o
}

func (o *OrgManager) loadFromStore() {
	entries, err := o.store.List("org:")
	if err != nil {
		utils.LogWarn("failed to load orgs from store: %v", err)
		return
	}
	for _, raw := range entries {
		var org Organization
		if err := json.Unmarshal(raw, &org); err != nil {
			continue
		}
		o.orgs[org.ID] = &org
	}
	utils.LogInfo("loaded %d organizations from persistent store", len(o.orgs))
}

func (o *OrgManager) persist(org *Organization) {
	if o.store == nil {
		return
	}
	data, err := json.Marshal(org)
	if err != nil {
		return
	}
	o.store.Set("org:"+org.ID, data)
}

func (o *OrgManager) Create(name string) *Organization {
	org := &Organization{
		ID:         utils.ShortHash(utils.HashBytes([]byte(name+time.Now().String())), 12),
		Name:       name,
		InviteCode: utils.ShortHash(utils.HashBytes([]byte(name+"invite"+time.Now().String())), 8),
		CreatedAt:  time.Now(),
	}
	o.mu.Lock()
	o.orgs[org.ID] = org
	o.mu.Unlock()
	o.persist(org)
	return org
}

func (o *OrgManager) JoinByInviteCode(inviteCode, peerID string) (*Organization, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, org := range o.orgs {
		if org.InviteCode == inviteCode {
			for _, m := range org.MemberIDs {
				if m == peerID {
					return org, true
				}
			}
			org.MemberIDs = append(org.MemberIDs, peerID)
			o.persist(org)
			return org, true
		}
	}
	return nil, false
}

func (o *OrgManager) List() []*Organization {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result := make([]*Organization, 0, len(o.orgs))
	for _, org := range o.orgs {
		result = append(result, org)
	}
	return result
}

func (o *OrgManager) Get(id string) (*Organization, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	org, ok := o.orgs[id]
	return org, ok
}
