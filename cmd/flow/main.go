package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Natarizki/flow/internal/api"
	"github.com/Natarizki/flow/internal/auth"
	"github.com/Natarizki/flow/internal/cache"
	"github.com/Natarizki/flow/internal/compression"
	"github.com/Natarizki/flow/internal/config"
	"github.com/Natarizki/flow/internal/enterprise"
	"github.com/Natarizki/flow/internal/fetcher"
	"github.com/Natarizki/flow/internal/network"
	"github.com/Natarizki/flow/internal/p2p"
	"github.com/Natarizki/flow/internal/prefetch"
	"github.com/Natarizki/flow/internal/security"
	"github.com/Natarizki/flow/internal/social"
	"github.com/Natarizki/flow/internal/store"
	websocketpkg "github.com/Natarizki/flow/internal/websocket"
	"github.com/Natarizki/flow/pkg/utils"
)

func main() {
	utils.LogInfo("starting FLOW daemon...")

	cfg, err := utils.LoadConfig(".")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	identity, err := security.LoadOrCreateIdentity(filepath.Join(cfg.CacheDir, "identity.key"))
	if err != nil {
		log.Fatalf("failed to load identity: %v", err)
	}
	if cfg.NodeID == "" {
		cfg.NodeID = identity.FingerprintHex()
	}
	utils.LogInfo("node fingerprint: %s...", utils.ShortHash(identity.FingerprintHex(), 16))

	certPath := filepath.Join(cfg.CacheDir, "tls", "cert.pem")
	keyPath := filepath.Join(cfg.CacheDir, "tls", "key.pem")
	if err := security.GenerateSelfSignedCert(certPath, keyPath); err != nil {
		utils.LogWarn("failed to generate TLS cert: %v", err)
	}

	certRotator, err := security.NewCertRotator(certPath, keyPath, 30*24*time.Hour)
	if err != nil {
		utils.LogWarn("failed to init cert rotator: %v", err)
	}
	stopCertRotation := make(chan struct{})
	if certRotator != nil {
		go certRotator.Run(stopCertRotation)
	}

	phishingList := security.NewPhishingBlacklist(filepath.Join(cfg.CacheDir, "phishing-blocklist.txt"))
	malwareScanner := security.NewMalwareScanner("tcp", "127.0.0.1:3310", false)

	var metaStore *store.Store
	if !cfg.IncognitoMode {
		metaDBPath := filepath.Join(cfg.CacheDir, "meta.db")
		metaStore, err = store.Open(metaDBPath)
		if err != nil {
			log.Fatalf("failed to open metadata store: %v", err)
		}
		defer metaStore.Close()
	} else {
		utils.LogInfo("incognito mode active: no data will be persisted to disk this session")
	}

	userStore := auth.NewUserStore(metaStore)
	sessionStore := auth.NewSessionStore(metaStore)
	peerManager := p2p.NewPeerManager(metaStore)
	tagManager := p2p.NewTagManager(metaStore)
	orgManager := p2p.NewOrgManager(metaStore)

	var index cache.IndexStore
	var blobStore cache.BlobStore
	if cfg.IncognitoMode {
		index = cache.NewIncognitoIndex(cfg.CacheMaxSize)
		blobStore = cache.NewIncognitoStorage()
	} else {
		diskStorage, err := cache.NewStorage(cfg.CacheDir)
		if err != nil {
			log.Fatalf("failed to init storage: %v", err)
		}
		blobStore = diskStorage
		diskIndex := cache.NewIndex(cfg.CacheMaxSize, diskStorage)
		index = diskIndex
	}

	chunkStore := p2p.NewChunkStore()

	leaderboard := social.NewLeaderboard(metaStore)
	achievements := social.NewAchievementManager(metaStore)
	community := social.NewCommunityManager(metaStore)
	bandwidthTracker := social.NewBandwidthTracker(metaStore)
	questManager := social.NewQuestManager(metaStore)
	uniquePeersServed := make(map[string]bool)
	var uniquePeersMu sync.Mutex

	license := enterprise.NewLicenseManager()
	mesh := enterprise.NewMeshController()
	analytics := enterprise.NewAnalytics(metaStore)

	flowFetcher := fetcher.NewFetcher(blobStore, index, chunkStore, compression.Level1, cfg.ContentBlindingEnabled)

	smartDNS := network.NewSmartDNS()
	chunkWorkerPool := p2p.NewAdaptiveWorkerPool(0, 32)
	predictor := prefetch.NewPredictor()

	hub := websocketpkg.NewHub()
	handler := websocketpkg.NewHandler(hub)

	selfURL := fmt.Sprintf("ws://localhost:%d/ws", cfg.DaemonPort)
	dht := p2p.NewDHTNodeFromPubKey(identity.PublicKey, selfURL, hub)
	handler.OnDHTMessage(dht.HandleMessage)

	flowFetcher.SetP2PLookup(func(url string, level compression.QuantLevel) (*fetcher.FetchResult, bool) {
		cacheKey := utils.HashBytes([]byte(fmt.Sprintf("%s|%d", url, level)))
		result, ok := p2p.TryFetchFromNetwork(hub, peerManager, dht, chunkStore, blobStore, cfg.NodeID, cacheKey)
		if !ok {
			return nil, false
		}
		return &fetcher.FetchResult{
			URL: url, Hash: result.Hash, CompressedSize: result.CompressedSize,
			Level: level, FromCache: false, ChunkHashes: result.ChunkHashes,
		}, true
	})

	handler.OnHandshake(func(c *websocketpkg.Client, p websocketpkg.HandshakePayload) {
		utils.LogInfo("handshake completed with peer %s (pubkey: %s...)", p.PeerID, utils.ShortHash(p.PublicKey, 8))
		dht.RegisterContact(p.PeerID, c.PeerID)
		questManager.RecordProgress(cfg.NodeID, "connect_3_peers", 1)
	})

	handler.OnPeerAnnounce(func(c *websocketpkg.Client, p websocketpkg.PeerAnnouncePayload) {
		peerManager.Add(&p2p.Peer{
			ID:         p.PeerID,
			Address:    p.Address,
			Port:       p.Port,
			Tags:       p.Tags,
			Visibility: p2p.PeerVisibility(p.Visible),
		})
	})

	handler.OnChunkHave(func(c *websocketpkg.Client, p websocketpkg.ChunkHavePayload) {
		for _, chunkHash := range p.ChunkHashes {
			dht.RecordProvider(chunkHash, c.PeerID)
		}
	})

	handler.OnChunkResponse(func(c *websocketpkg.Client, p websocketpkg.ChunkResponsePayload) {
		utils.LogWarn("received unsolicited chunk_response from %s (chunk %s)", c.PeerID, utils.ShortHash(p.ChunkHash, 8))
	})

	handler.OnChunkRequest(func(c *websocketpkg.Client, msg *websocketpkg.Message, p websocketpkg.ChunkRequestPayload) {
		chunkWorkerPool.Submit(func() {
			chunk, err := chunkStore.Get(p.FileHash, p.ChunkHash)
			if err != nil {
				utils.LogWarn("chunk request failed: %v", err)
				return
			}
			payload := websocketpkg.ChunkResponsePayload{
				ChunkHash: chunk.Hash,
				Data:      chunk.Data,
				Checksum:  utils.HashBytes(chunk.Data),
			}
			respMsg, err := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeChunkResponse, msg.ID, "", payload)
			if err != nil {
				return
			}
			c.SendMessage(respMsg)

			leaderboard.RecordContribution(cfg.NodeID, "", int64(len(chunk.Data)))
			bandwidthTracker.RecordUpload(int64(len(chunk.Data)))
			questManager.RecordProgress(cfg.NodeID, "serve_10_chunks", 1)
			questManager.RecordProgress(cfg.NodeID, "serve_50_chunks", 1)

			uniquePeersMu.Lock()
			uniquePeersServed[c.PeerID] = true
			peerCount := len(uniquePeersServed)
			uniquePeersMu.Unlock()

			rank, entry := leaderboard.GetRank(cfg.NodeID)
			var bytesServed, chunksServed int64
			if entry != nil {
				bytesServed = entry.BytesServed
				chunksServed = entry.ChunksServed
			}
			cacheCount, _ := index.Stats()

			achievements.CheckAndUnlock(cfg.NodeID, social.Stats{
				ChunksServed:    chunksServed,
				BytesServed:     bytesServed,
				CacheEntries:    cacheCount,
				UniquePeersSeen: peerCount,
				PrefetchEnabled: predictor.IsEnabled(),
				LeaderboardRank: rank,
			})
		})
	})

	handler.OnManifestRequest(func(c *websocketpkg.Client, msg *websocketpkg.Message, p websocketpkg.ManifestRequestPayload) {
		hashes := chunkStore.HaveChunks(p.FileHash)
		resp := websocketpkg.ManifestResponsePayload{
			FileHash:    p.FileHash,
			ChunkHashes: hashes,
			Found:       len(hashes) > 0,
		}
		respMsg, err := websocketpkg.NewMessageWithID(websocketpkg.MsgTypeManifestResponse, msg.ID, "", resp)
		if err != nil {
			return
		}
		c.SendMessage(respMsg)
	})

	go hub.Run()

	connector := p2p.NewConnector(hub, handler, cfg.NodeID, identity.FingerprintHex())
	connector.SetMeshAwareness(func(peerID string) bool {
		return mesh.SameMesh(cfg.NodeID, peerID)
	})

	stopWatch := make(chan struct{})
	go connector.WatchPeerManager(peerManager, 15*time.Second, stopWatch)

	if cfg.TrackerURL != "" {
		connector.EnsureConnected(cfg.TrackerURL)
	}

	lanSync := p2p.NewLANSync(cfg.NodeID, cfg.NodeID, cfg.DaemonPort, cfg.DashboardPort)
	lanSync.OnPeerDiscovered(func(ann p2p.LANAnnouncement, sourceIP string) {
		utils.LogInfo("lan sync: discovered peer %s at %s:%d", ann.PeerName, sourceIP, ann.WSPort)
		connector.EnsureConnected(fmt.Sprintf("ws://%s:%d/ws", sourceIP, ann.WSPort))
	})
	stopLANSync := make(chan struct{})
	go lanSync.Broadcast(10*time.Second, stopLANSync)
	go lanSync.Listen(stopLANSync)

	scheduler := prefetch.NewScheduler(predictor, func(url string) error {
		if phishingList.IsBlocked(url) {
			return fmt.Errorf("refusing to prefetch blacklisted URL: %s", url)
		}

		result, err := flowFetcher.Fetch(url)
		if err != nil {
			return err
		}

		if malwareScanner.Enabled() {
			if data, rerr := flowFetcher.ReadDecoded(result.Hash); rerr == nil {
				if scanResult, serr := malwareScanner.Scan(data); serr == nil && !scanResult.Clean {
					utils.LogWarn("malware detected in prefetched content from %s: %s", url, scanResult.SignatureName)
					return fmt.Errorf("prefetched content flagged as malware: %s", scanResult.SignatureName)
				}
			}
		}

		dht.Provide(result.Hash)
		bandwidthTracker.RecordDownload(result.CompressedSize)
		questManager.RecordProgress(cfg.NodeID, "fetch_5_pages", 1)
		if result.Level >= compression.Level3 {
			questManager.RecordProgress(cfg.NodeID, "compress_level3", 1)
		}

		if len(result.ChunkHashes) > 0 {
			havePayload := websocketpkg.ChunkHavePayload{FileHash: result.Hash, ChunkHashes: result.ChunkHashes}
			haveMsg, merr := websocketpkg.NewMessage(websocketpkg.MsgTypeChunkHave, cfg.NodeID, havePayload)
			if merr == nil {
				hub.Broadcast(haveMsg)
			}
		}
		return nil
	})

	stopPrefetch := make(chan struct{})
	go scheduler.RunBackground(predictor.ActiveSessions, 30*time.Second, stopPrefetch)

	stopAnalytics := make(chan struct{})
	go analytics.RunPeriodicSnapshot(func() enterprise.Snapshot {
		count, size := index.Stats()
		_, entry := leaderboard.GetRank(cfg.NodeID)
		var bytesServed, chunksServed int64
		if entry != nil {
			bytesServed = entry.BytesServed
			chunksServed = entry.ChunksServed
		}
		return enterprise.Snapshot{
			Timestamp: time.Now(), PeerCount: hub.PeerCount(),
			CacheEntries: count, CacheSize: size,
			BytesServed: bytesServed, ChunksServed: chunksServed,
		}
	}, 1*time.Hour, stopAnalytics)

	stopConfigWatch := make(chan struct{})
	if !cfg.IncognitoMode {
		watcher := config.NewWatcher("./flow.yaml", 5*time.Second)
		go watcher.Run(func(newCfg *utils.Config) {
			cfg.ApplyHotReload(newCfg, func(newSize int64) {
				if idx, ok := index.(*cache.Index); ok {
					idx.SetMaxSize(newSize)
				}
			}, func(newBlinding bool) {
				flowFetcher.SetBlindingEnabled(newBlinding)
			})
		}, stopConfigWatch)
	}

	bookmarkStore := social.NewBookmarkStore(metaStore)

	videoPreroller := prefetch.NewVideoPreroller(func(url string, data []byte, contentType string) error {
		_, err := flowFetcher.StoreRaw(url, data, contentType, compression.LevelLossless)
		return err
	})

	wikiPrecacher := prefetch.NewWikipediaPrecacher(func(url string) error {
		_, err := flowFetcher.Fetch(url)
		return err
	})

	apiServer := api.NewServer(
		hub, peerManager, index, blobStore, predictor, scheduler,
		userStore, sessionStore, leaderboard, achievements, community,
		flowFetcher, license, mesh, analytics,
		tagManager, orgManager, bandwidthTracker, questManager, smartDNS,
		func(wsURL string) { connector.EnsureConnected(wsURL) },
		videoPreroller, bookmarkStore, wikiPrecacher,
	)
	apiServer.Handler().HandleFunc("/ws", websocketpkg.ServeWS(hub, handler))
	api.RegisterPprof(apiServer.Handler())
	apiServer.Handler().Handle("/", api.ServeDashboard("./web/dashboard"))

	apiAddr := fmt.Sprintf(":%d", cfg.DashboardPort)
	go func() {
		if err := apiServer.ListenAndServe(apiAddr); err != nil {
			utils.LogError("api server error: %v", err)
		}
	}()

	utils.LogInfo("FLOW daemon running — node_id=%s dashboard=%s cache_dir=%s tracker=%s incognito=%v",
		cfg.NodeID, apiAddr, cfg.CacheDir, cfg.TrackerURL, cfg.IncognitoMode)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	close(stopWatch)
	close(stopPrefetch)
	close(stopAnalytics)
	close(stopCertRotation)
	close(stopLANSync)
	close(stopConfigWatch)
	chunkWorkerPool.Shutdown()
	connector.StopAll()
	dht.Close()

	if cfg.IncognitoMode {
		if incogIdx, ok := index.(*cache.IncognitoIndex); ok {
			incogIdx.Purge()
		}
		if incogStore, ok := blobStore.(*cache.IncognitoStorage); ok {
			incogStore.Purge()
		}
		utils.LogInfo("incognito session data purged")
	}

	utils.LogInfo("FLOW daemon shutting down gracefully...")
}
