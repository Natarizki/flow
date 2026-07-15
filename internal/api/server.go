package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Natarizki/flow/internal/auth"
	"github.com/Natarizki/flow/internal/cache"
	"github.com/Natarizki/flow/internal/enterprise"
	"github.com/Natarizki/flow/internal/fetcher"
	"github.com/Natarizki/flow/internal/network"
	"github.com/Natarizki/flow/internal/p2p"
	"github.com/Natarizki/flow/internal/prefetch"
	"github.com/Natarizki/flow/internal/social"
	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

type Server struct {
	hub               *websocketpkg.Hub
	peers             *p2p.PeerManager
	index             cache.IndexStore
	storage           cache.BlobStore
	users             *auth.UserStore
	sessions          *auth.SessionStore
	predictor         *prefetch.Predictor
	scheduler         *prefetch.Scheduler
	leaderboard       *social.Leaderboard
	achievements      *social.AchievementManager
	community         *social.CommunityManager
	fetcher           *fetcher.Fetcher
	license           *enterprise.LicenseManager
	mesh              *enterprise.MeshController
	analytics         *enterprise.Analytics
	tags              *p2p.TagManager
	orgs              *p2p.OrgManager
	bandwidth         *social.BandwidthTracker
	quests            *social.QuestManager
	dns               *network.SmartDNS
	wifiDirectConnect func(wsURL string)
	videoPreroller    *prefetch.VideoPreroller
	bookmarks         *social.BookmarkStore
	wikiPrecacher     *prefetch.WikipediaPrecacher
	loginLimiter      *loginRateLimiter
	mux               *http.ServeMux
}

func NewServer(
	hub *websocketpkg.Hub,
	peers *p2p.PeerManager,
	index cache.IndexStore,
	storage cache.BlobStore,
	predictor *prefetch.Predictor,
	scheduler *prefetch.Scheduler,
	users *auth.UserStore,
	sessions *auth.SessionStore,
	leaderboard *social.Leaderboard,
	achievements *social.AchievementManager,
	community *social.CommunityManager,
	flowFetcher *fetcher.Fetcher,
	license *enterprise.LicenseManager,
	mesh *enterprise.MeshController,
	analytics *enterprise.Analytics,
	tags *p2p.TagManager,
	orgs *p2p.OrgManager,
	bandwidth *social.BandwidthTracker,
	quests *social.QuestManager,
	dns *network.SmartDNS,
	wifiDirectConnect func(wsURL string),
	videoPreroller *prefetch.VideoPreroller,
	bookmarks *social.BookmarkStore,
	wikiPrecacher *prefetch.WikipediaPrecacher,
) *Server {
	s := &Server{
		hub:               hub,
		peers:             peers,
		index:             index,
		storage:           storage,
		users:             users,
		sessions:          sessions,
		predictor:         predictor,
		scheduler:         scheduler,
		leaderboard:       leaderboard,
		achievements:      achievements,
		community:         community,
		fetcher:           flowFetcher,
		license:           license,
		mesh:              mesh,
		analytics:         analytics,
		tags:              tags,
		orgs:              orgs,
		bandwidth:         bandwidth,
		quests:            quests,
		dns:               dns,
		wifiDirectConnect: wifiDirectConnect,
		videoPreroller:    videoPreroller,
		bookmarks:         bookmarks,
		wikiPrecacher:     wikiPrecacher,
		loginLimiter:      newLoginRateLimiter(5, 5*time.Minute, 15*time.Minute),
		mux:               http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) protected(next http.HandlerFunc) http.Handler {
	checked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		authHeader := r.Header.Get("Authorization")
		token := ""
		if len(authHeader) > 7 {
			token = authHeader[7:]
		}
		if !s.sessions.IsActive(claims.UserID, token) {
			writeError(w, http.StatusUnauthorized, "session revoked, please login again")
			return
		}
		next(w, r)
	})
	return auth.Middleware(checked)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/auth/register", s.handleRegister)
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/license/status", s.handleLicenseStatus)

	s.mux.Handle("/api/auth/logout", s.protected(s.handleLogout))
	s.mux.Handle("/api/auth/status", s.protected(s.handleAuthStatus))
	s.mux.Handle("/api/auth/refresh", s.protected(s.handleRefresh))

	s.mux.Handle("/api/stats", s.protected(s.handleStats))

	s.mux.Handle("/api/peers", s.protected(s.handlePeersCollection))
	s.mux.Handle("/api/peers/", s.protected(s.handlePeerItem))

	s.mux.Handle("/api/cache", s.protected(s.handleCacheList))
	s.mux.Handle("/api/cache/clean", s.protected(s.handleCacheClean))
	s.mux.Handle("/api/cache/export", s.protected(s.handleCacheExport))
	s.mux.Handle("/api/cache/import", s.protected(s.handleCacheImport))
	s.mux.Handle("/api/cache/read", s.protected(s.handleCacheRead))

	s.mux.Handle("/api/prefetch/train", s.protected(s.handlePrefetchTrain))
	s.mux.Handle("/api/prefetch/predict", s.protected(s.handlePrefetchPredict))
	s.mux.Handle("/api/prefetch/enable", s.protected(s.handlePrefetchEnable))
	s.mux.Handle("/api/prefetch/disable", s.protected(s.handlePrefetchDisable))
	s.mux.Handle("/api/prefetch/now", s.protected(s.handlePrefetchNow))
	s.mux.Handle("/api/prefetch/record", s.protected(s.handlePrefetchRecord))

	s.mux.Handle("/api/leaderboard", s.protected(s.handleLeaderboardGlobal))
	s.mux.Handle("/api/achievements", s.protected(s.handleAchievementsList))
	s.mux.Handle("/api/community/events", s.protected(s.handleCommunityEventsList))
	s.mux.Handle("/api/community/events/create", s.protected(s.handleCommunityEventCreate))
	s.mux.Handle("/api/community/events/join", s.protected(s.handleCommunityEventJoin))

	s.mux.Handle("/api/license/activate", s.protected(s.handleLicenseActivate))
	s.mux.Handle("/api/enterprise/mesh", s.protected(s.requireEnterprise(s.handleMeshList)))
	s.mux.Handle("/api/enterprise/mesh/create", s.protected(s.requireEnterprise(s.handleMeshCreate)))
	s.mux.Handle("/api/enterprise/mesh/add-member", s.protected(s.requireEnterprise(s.handleMeshAddMember)))
	s.mux.Handle("/api/enterprise/analytics", s.protected(s.requireEnterprise(s.handleAnalyticsList)))
	s.mux.Handle("/api/enterprise/analytics/export", s.protected(s.requireEnterprise(s.handleAnalyticsExportCSV)))

	s.mux.Handle("/api/whois", s.protected(s.handleWhois))
	s.mux.Handle("/api/discover/lan", s.protected(s.handleDiscoverLAN))
	s.mux.Handle("/api/discover/org", s.protected(s.handleDiscoverOrg))
	s.mux.Handle("/api/bandwidth/today", s.protected(s.handleBandwidthToday))
	s.mux.Handle("/api/bandwidth/month", s.protected(s.handleBandwidthMonth))
	s.mux.Handle("/api/tags/add", s.protected(s.handleTagAdd))
	s.mux.Handle("/api/tags/remove", s.protected(s.handleTagRemove))
	s.mux.Handle("/api/tags/list", s.protected(s.handleTagList))
	s.mux.Handle("/api/org/create", s.protected(s.handleOrgCreate))
	s.mux.Handle("/api/org/join", s.protected(s.handleOrgJoin))
	s.mux.Handle("/api/org/list", s.protected(s.handleOrgList))

	s.mux.Handle("/api/quests/today", s.protected(s.handleQuestsToday))
	s.mux.Handle("/api/quests/progress", s.protected(s.handleQuestsProgress))

	s.mux.Handle("/api/dns/resolve", s.protected(s.handleDNSResolve))

	s.mux.Handle("/api/wifi-direct/discovered", s.protected(s.handleWifiDirectDiscovered))
	s.mux.Handle("/api/wifi-direct/connected", s.protected(s.handleWifiDirectConnected))

	s.mux.Handle("/api/video/preroll", s.protected(s.handleVideoPrerollPage))
	s.mux.Handle("/api/bookmarks/add", s.protected(s.handleBookmarkAdd))
	s.mux.Handle("/api/bookmarks/remove", s.protected(s.handleBookmarkRemove))
	s.mux.Handle("/api/bookmarks", s.protected(s.handleBookmarkList))
	s.mux.Handle("/api/bookmarks/sync/export", s.protected(s.handleBookmarkSyncExport))
	s.mux.Handle("/api/bookmarks/sync/merge", s.protected(s.handleBookmarkSyncMerge))
	s.mux.Handle("/api/wikipedia/precache", s.protected(s.handleWikipediaPrecache))
}

func (s *Server) Handler() *http.ServeMux { return s.mux }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	count, size := s.index.Stats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"peer_count":  s.hub.PeerCount(),
		"cache_count": count,
		"cache_size":  size,
	})
}

func (s *Server) ListenAndServe(addr string) error {
	utils.LogInfo("api server listening on %s", addr)
	return http.ListenAndServe(addr, corsMiddleware(s.mux))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeBody(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(v)
}
