package enterprise

import (
	"sync"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// MeshController mengelompokkan peer jadi satu "mesh" logis buat
// organisasi — dipakai buat prioritas routing (peer di mesh yang sama
// diprioritaskan buat P2P chunk transfer dibanding peer random di luar
// organisasi) dan buat scoping visibility "internal".
type Mesh struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OrgName   string    `json:"org_name"`
	MemberIDs []string  `json:"member_ids"`
	CreatedAt time.Time `json:"created_at"`
	Priority  int       `json:"priority"` // makin tinggi, makin diprioritaskan routing-nya
}

type MeshController struct {
	meshes map[string]*Mesh
	mu     sync.RWMutex
}

func NewMeshController() *MeshController {
	return &MeshController{meshes: make(map[string]*Mesh)}
}

func (mc *MeshController) CreateMesh(id, name, orgName string, priority int) *Mesh {
	m := &Mesh{
		ID:        id,
		Name:      name,
		OrgName:   orgName,
		CreatedAt: time.Now(),
		Priority:  priority,
	}
	mc.mu.Lock()
	mc.meshes[id] = m
	mc.mu.Unlock()
	utils.LogInfo("enterprise: created mesh '%s' for org '%s'", name, orgName)
	return m
}

func (mc *MeshController) AddMember(meshID, peerID string) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	m, ok := mc.meshes[meshID]
	if !ok {
		return false
	}
	for _, id := range m.MemberIDs {
		if id == peerID {
			return true
		}
	}
	m.MemberIDs = append(m.MemberIDs, peerID)
	return true
}

func (mc *MeshController) RemoveMember(meshID, peerID string) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	m, ok := mc.meshes[meshID]
	if !ok {
		return false
	}
	for i, id := range m.MemberIDs {
		if id == peerID {
			m.MemberIDs = append(m.MemberIDs[:i], m.MemberIDs[i+1:]...)
			return true
		}
	}
	return false
}

// MeshOf cari mesh mana yang isinya peerID ini — dipakai routing logic
// buat cek "apakah 2 peer ini satu organisasi?"
func (mc *MeshController) MeshOf(peerID string) (*Mesh, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for _, m := range mc.meshes {
		for _, id := range m.MemberIDs {
			if id == peerID {
				return m, true
			}
		}
	}
	return nil, false
}

// SameMesh dua peer dibilang "sama mesh" kalau ada minimal satu mesh yang
// isinya keduanya — dipakai buat prioritize routing di connector/DHT.
func (mc *MeshController) SameMesh(peerIDA, peerIDB string) bool {
	meshA, okA := mc.MeshOf(peerIDA)
	if !okA {
		return false
	}
	meshB, okB := mc.MeshOf(peerIDB)
	return okB && meshA.ID == meshB.ID
}

func (mc *MeshController) List() []*Mesh {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	result := make([]*Mesh, 0, len(mc.meshes))
	for _, m := range mc.meshes {
		result = append(result, m)
	}
	return result
}

func (mc *MeshController) Get(id string) (*Mesh, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	m, ok := mc.meshes[id]
	return m, ok
}
