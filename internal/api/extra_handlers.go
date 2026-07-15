package api

import "net/http"

func (s *Server) handleWhois(w http.ResponseWriter, r *http.Request) {
	peerName := r.URL.Query().Get("peer")
	if peerName == "" {
		writeError(w, http.StatusBadRequest, "peer query param required")
		return
	}
	peer, ok := findPeerByName(s.peers, peerName)
	if !ok {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	rank, entry := s.leaderboard.GetRank(peer.ID)
	tags := s.tags.TagsOf(peer.ID)

	resp := map[string]interface{}{
		"peer":            peer,
		"tags":            tags,
		"leaderboard_rank": rank,
	}
	if entry != nil {
		resp["bytes_served"] = entry.BytesServed
		resp["chunks_served"] = entry.ChunksServed
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDiscoverLAN(w http.ResponseWriter, r *http.Request) {
	// "LAN" discovery = peers we know about whose address looks like a
	// private/local subnet — real filter over real PeerManager data,
	// not a fake broadcast (actual mDNS/UDP broadcast discovery is a
	// separate subsystem, this endpoint reports what's already known).
	var lanPeers []interface{}
	for _, p := range s.peers.List() {
		if isPrivateAddress(p.Address) {
			lanPeers = append(lanPeers, p)
		}
	}
	writeJSON(w, http.StatusOK, lanPeers)
}

func isPrivateAddress(addr string) bool {
	prefixes := []string{"127.", "10.", "192.168.", "172.16.", "172.17.", "172.18.", "172.19.",
		"172.2", "172.30.", "172.31.", "localhost", "::1"}
	for _, p := range prefixes {
		if len(addr) >= len(p) && addr[:len(p)] == p {
			return true
		}
	}
	return false
}

func (s *Server) handleDiscoverOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org query param required")
		return
	}
	org, ok := s.orgs.Get(orgID)
	if !ok {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}
	var members []interface{}
	for _, memberID := range org.MemberIDs {
		if peer, ok := s.peers.Get(memberID); ok {
			members = append(members, peer)
		}
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleBandwidthToday(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.bandwidth.Today())
}

func (s *Server) handleBandwidthMonth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.bandwidth.ThisMonth())
}

func (s *Server) handleTagAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PeerName string `json:"peer_name"`
		Tag      string `json:"tag"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	peer, ok := findPeerByName(s.peers, req.PeerName)
	if !ok {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	s.tags.Add(peer.ID, req.Tag)
	writeJSON(w, http.StatusOK, map[string]string{"status": "tagged"})
}

func (s *Server) handleTagRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PeerName string `json:"peer_name"`
		Tag      string `json:"tag"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	peer, ok := findPeerByName(s.peers, req.PeerName)
	if !ok {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}
	s.tags.Remove(peer.ID, req.Tag)
	writeJSON(w, http.StatusOK, map[string]string{"status": "untagged"})
}

func (s *Server) handleTagList(w http.ResponseWriter, r *http.Request) {
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag query param required")
		return
	}
	peerIDs := s.tags.PeersWithTag(tag)
	var peers []interface{}
	for _, id := range peerIDs {
		if p, ok := s.peers.Get(id); ok {
			peers = append(peers, p)
		}
	}
	writeJSON(w, http.StatusOK, peers)
}

func (s *Server) handleOrgCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	org := s.orgs.Create(req.Name)
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) handleOrgJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InviteCode string `json:"invite_code"`
		PeerID     string `json:"peer_id"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	org, ok := s.orgs.JoinByInviteCode(req.InviteCode, req.PeerID)
	if !ok {
		writeError(w, http.StatusNotFound, "invalid invite code")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (s *Server) handleOrgList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.orgs.List())
}
