const REFRESH_INTERVAL = 5000;
const TOKEN_KEY = "flow_token";

// ===== token helpers =====

function getToken() {
	return localStorage.getItem(TOKEN_KEY);
}
function setToken(token) {
	localStorage.setItem(TOKEN_KEY, token);
}
function clearToken() {
	localStorage.removeItem(TOKEN_KEY);
}

async function apiFetch(path, options = {}) {
	const headers = options.headers || {};
	const token = getToken();
	if (token) headers["Authorization"] = "Bearer " + token;
	if (options.body) headers["Content-Type"] = "application/json";

	const res = await fetch(path, { ...options, headers });
	if (res.status === 401) {
		clearToken();
		showLogin();
		throw new Error("session expired");
	}
	if (!res.ok) {
		const body = await res.json().catch(() => ({}));
		throw new Error(body.error || `request failed (${res.status})`);
	}
	return res.status === 204 ? null : res.json();
}

// ===== login / register UI =====

const loginScreen = document.getElementById("login-screen");
const app = document.getElementById("app");
const loginForm = document.getElementById("login-form");
const registerForm = document.getElementById("register-form");
const loginError = document.getElementById("login-error");
const formTitle = document.getElementById("form-title");

document.getElementById("switch-to-register").addEventListener("click", (e) => {
	e.preventDefault();
	loginForm.style.display = "none";
	registerForm.style.display = "block";
	document.getElementById("switch-to-register").style.display = "none";
	document.getElementById("switch-to-login").style.display = "block";
	formTitle.textContent = "Create your account";
	hideError();
});

document.getElementById("switch-to-login").addEventListener("click", (e) => {
	e.preventDefault();
	registerForm.style.display = "none";
	loginForm.style.display = "block";
	document.getElementById("switch-to-login").style.display = "none";
	document.getElementById("switch-to-register").style.display = "block";
	formTitle.textContent = "Sign in to FLOW";
	hideError();
});

function showError(msg) {
	loginError.textContent = msg;
	loginError.style.display = "block";
}
function hideError() {
	loginError.style.display = "none";
}

loginForm.addEventListener("submit", async (e) => {
	e.preventDefault();
	hideError();
	const email = document.getElementById("login-email").value;
	const password = document.getElementById("login-password").value;

	try {
		const resp = await apiFetch("/api/auth/login", {
			method: "POST",
			body: JSON.stringify({ email, password }),
		});
		setToken(resp.token);
		enterApp(resp.user);
	} catch (err) {
		showError(err.message || "login failed");
	}
});

registerForm.addEventListener("submit", async (e) => {
	e.preventDefault();
	hideError();
	const email = document.getElementById("reg-email").value;
	const username = document.getElementById("reg-username").value;
	const password = document.getElementById("reg-password").value;

	try {
		await apiFetch("/api/auth/register", {
			method: "POST",
			body: JSON.stringify({ email, username, password }),
		});
		// auto-switch to login after successful register
		document.getElementById("switch-to-login").click();
		document.getElementById("login-email").value = email;
		showError("Account created — please sign in.");
		loginError.style.background = "rgba(63,185,80,0.1)";
		loginError.style.borderColor = "rgba(63,185,80,0.4)";
		loginError.style.color = "var(--green)";
	} catch (err) {
		showError(err.message || "registration failed");
	}
});

function showLogin() {
	loginScreen.style.display = "flex";
	app.style.display = "none";
}

async function enterApp(userHint) {
	loginScreen.style.display = "none";
	app.style.display = "block";

	let user = userHint;
	if (!user) {
		try {
			user = await apiFetch("/api/auth/status");
		} catch {
			return; // apiFetch already redirected to login on 401
		}
	}

	const displayName = user.username || user.Username || user.email || user.Email || "?";
	document.getElementById("user-avatar").textContent = displayName[0].toUpperCase();
	document.getElementById("user-email-display").textContent = user.email || user.Email || "";
	document.getElementById("stat-username").textContent = displayName;
        
        const token = getToken();
	if (token) {
		registerFlowServiceWorker(token).catch(() => {});
	}

	refreshAll();
}

// ===== logout / dropdown =====

document.getElementById("user-avatar").addEventListener("click", () => {
	document.getElementById("user-dropdown").classList.toggle("open");
});
document.addEventListener("click", (e) => {
	if (!e.target.closest(".user-menu")) {
		document.getElementById("user-dropdown").classList.remove("open");
	}
});
document.getElementById("logout-btn").addEventListener("click", async (e) => {
	e.preventDefault();
	try {
		await apiFetch("/api/auth/logout", { method: "POST" });
	} catch {}
	clearToken();
	location.reload();
});

// ===== tab switching =====

document.querySelectorAll(".topnav-link").forEach(link => {
	link.addEventListener("click", (e) => {
		e.preventDefault();
		document.querySelectorAll(".topnav-link").forEach(l => l.classList.remove("active"));
		document.querySelectorAll(".tab-content").forEach(t => t.style.display = "none");
		link.classList.add("active");
		document.getElementById("tab-" + link.dataset.tab).style.display = "block";

		switch (link.dataset.tab) {
			case "achievements": refreshAchievements(); break;
			case "quests": refreshQuests(); break;
			case "bookmarks": refreshBookmarks(); break;
			case "enterprise":
				refreshLicenseStatus();
				refreshMeshList();
				refreshAnalytics();
				break;
		}
                switch (link.dataset.tab) {
			case "achievements": refreshAchievements(); break;
			case "quests": refreshQuests(); break;
			case "bookmarks": refreshBookmarks(); break;
			case "events": refreshEvents(); break;
			case "enterprise":
				refreshLicenseStatus();
				refreshMeshList();
				refreshAnalytics();
				break;
		}
	});
});

// ===== formatting helpers =====

function formatBytes(bytes) {
	if (!bytes) return "0 B";
	const units = ["B", "KB", "MB", "GB"];
	let i = 0, val = bytes;
	while (val >= 1024 && i < units.length - 1) { val /= 1024; i++; }
	return `${val.toFixed(val < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

function timeAgo(isoString) {
	if (!isoString) return "-";
	const diff = (Date.now() - new Date(isoString).getTime()) / 1000;
	if (diff < 60) return `${Math.floor(diff)}s ago`;
	if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
	if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
	return `${Math.floor(diff / 86400)}d ago`;
}

function escapeHTML(str) {
	const div = document.createElement("div");
	div.textContent = str ?? "";
	return div.innerHTML;
}

// ===== data refresh =====

async function refreshHealth() {
	const badge = document.getElementById("status-badge");
	try {
		await fetch("/health").then(r => { if (!r.ok) throw new Error(); });
		badge.textContent = "online";
		badge.className = "badge badge-ok";
	} catch {
		badge.textContent = "offline";
		badge.className = "badge badge-error";
	}
}

async function refreshStats() {
	try {
		const stats = await apiFetch("/api/stats");
		document.getElementById("stat-peers").textContent = stats.peer_count ?? 0;
		document.getElementById("stat-cache-count").textContent = stats.cache_count ?? 0;
		document.getElementById("stat-cache-size").textContent = formatBytes(stats.cache_size);
	} catch (e) {
		console.error("stats refresh failed", e);
	}
}

async function refreshPeers() {
	const list = document.getElementById("peers-list");
	try {
		const peers = await apiFetch("/api/peers");
		document.getElementById("peers-count").textContent = `${peers?.length ?? 0} peers`;

		if (!peers || peers.length === 0) {
			list.innerHTML = `<li class="empty-row">No peers yet</li>`;
			return;
		}

		list.innerHTML = peers.map(p => {
			const name = p.Name || p.name || "unnamed";
			const id = (p.ID || p.id || "").slice(0, 20);
			const visibility = (p.Visibility || p.visibility || "private").toLowerCase();
			const reputation = (p.Reputation ?? p.reputation ?? 0).toFixed(1);
			const locked = p.Locked || p.locked;
			const safeId = (p.ID || p.id || "").replace(/[^a-zA-Z0-9]/g, "");

			return `
				<li class="repo-item">
					<div class="repo-main">
						<p class="repo-title">${escapeHTML(name)}
							<button class="readme-toggle-btn" data-peer-id="${escapeHTML(p.ID || p.id)}" data-target="readme-${safeId}">README</button>
						</p>
						<p class="repo-desc"><code>${escapeHTML(id)}</code></p>
						<div class="repo-meta">
							<span><span class="status-dot ${locked ? "status-locked" : "status-connected"}"></span>${locked ? "locked" : "active"}</span>
							<span>reputation ${reputation}</span>
						</div>
						<div id="readme-${safeId}" class="readme-container" style="display:none;"></div>
					</div>
					<span class="pill pill-${visibility}">${escapeHTML(visibility)}</span>
				</li>
			`;
		}).join("");

		// wire up README toggle buttons after render
		list.querySelectorAll(".readme-toggle-btn").forEach(btn => {
			btn.addEventListener("click", async () => {
				const peerId = btn.dataset.peerId;
				const targetId = btn.dataset.target;
				const container = document.getElementById(targetId);
				if (container.style.display === "none") {
					container.style.display = "block";
					container.innerHTML = "<p style='color:var(--text-dim);font-size:0.8rem;'>Loading README...</p>";
					try {
						const resp = await apiFetch(`/api/peers/${peerId}/readme`);
						if (!resp.readme) {
							container.innerHTML = "<p style='color:var(--text-dim);font-size:0.8rem;'>No README set for this peer.</p>";
						} else {
							await renderReadme(resp.readme, resp.readme_format || "md", container);
						}
					} catch (e) {
						container.innerHTML = "<p style='color:var(--red);font-size:0.8rem;'>Failed to load README.</p>";
					}
				} else {
					container.style.display = "none";
				}
			});
		});
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load peers</li>`;
	}
}

async function refreshCache() {
	const list = document.getElementById("cache-list");
	try {
		const entries = await apiFetch("/api/cache");
		document.getElementById("cache-count").textContent = `${entries?.length ?? 0} entries`;

		if (!entries || entries.length === 0) {
			list.innerHTML = `<li class="empty-row">No cache entries yet</li>`;
			return;
		}

		list.innerHTML = entries.map(e => {
			const hash = (e.Hash || e.hash || "").slice(0, 20);
			const url = e.URL || e.url || "(no url)";
			const size = formatBytes(e.Size || e.size);
			const level = e.QuantLevel ?? e.quant_level ?? 0;
			const accessed = timeAgo(e.LastAccess || e.last_access);

			return `
				<li class="repo-item">
					<div class="repo-main">
						<p class="repo-title"><code>${escapeHTML(hash)}</code></p>
						<p class="repo-desc">${escapeHTML(url)}</p>
						<div class="repo-meta">
							<span>${size}</span>
							<span>level ${level}</span>
							<span>${accessed}</span>
						</div>
					</div>
				</li>
			`;
		}).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load cache entries</li>`;
	}
}

function refreshAll() {
	refreshHealth();
	refreshStats();
	refreshPeers();
	refreshCache();
}

document.getElementById("refresh-peers").addEventListener("click", refreshPeers);
document.getElementById("refresh-cache").addEventListener("click", refreshCache);

// ===== Achievements =====

async function refreshAchievements() {
	const list = document.getElementById("achievements-list");
	try {
		const status = await apiFetch("/api/auth/status");
		const peerID = status.user_id;
		const resp = await apiFetch(`/api/achievements?peer_id=${peerID}`);
		const unlocked = resp.unlocked || [];
		const catalog = resp.catalog || [];

		document.getElementById("achievements-count").textContent = `${unlocked.length} / ${catalog.length} unlocked`;

		const unlockedIds = new Set(unlocked.map(b => b.id || b.ID));
		list.innerHTML = catalog.map(b => {
			const isUnlocked = unlockedIds.has(b.id || b.ID);
			const tier = b.tier || b.Tier || "bronze";
			return `
				<li class="repo-item" style="opacity:${isUnlocked ? "1" : "0.4"};">
					<div class="repo-main">
						<p class="repo-title">${isUnlocked ? "✓" : "🔒"} ${escapeHTML(b.name || b.Name)}</p>
						<p class="repo-desc">${escapeHTML(b.description || b.Description)}</p>
					</div>
					<span class="pill pill-${tier === "gold" || tier === "platinum" ? "public" : "internal"}">${escapeHTML(tier)}</span>
				</li>
			`;
		}).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load achievements</li>`;
	}
}

// ===== Quests =====

async function refreshQuests() {
	const list = document.getElementById("quests-list");
	try {
		const quests = await apiFetch("/api/quests/today");
		if (!quests || quests.length === 0) {
			list.innerHTML = `<li class="empty-row">No quests today</li>`;
			return;
		}
		list.innerHTML = quests.map(q => `
			<li class="repo-item">
				<div class="repo-main">
					<p class="repo-title">${escapeHTML(q.description || q.Description)}</p>
					<p class="repo-desc">Target: ${q.target ?? q.Target} — Reward: ${q.reward_score ?? q.RewardScore} pts</p>
				</div>
			</li>
		`).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load quests</li>`;
	}
}

// ===== Bookmarks =====

async function refreshBookmarks() {
	const list = document.getElementById("bookmarks-list");
	try {
		const bookmarks = await apiFetch("/api/bookmarks");
		document.getElementById("bookmarks-count").textContent = `${bookmarks?.length ?? 0} bookmarks`;

		if (!bookmarks || bookmarks.length === 0) {
			list.innerHTML = `<li class="empty-row">No bookmarks yet</li>`;
			return;
		}
		list.innerHTML = bookmarks.map(b => `
			<li class="repo-item">
				<div class="repo-main">
					<p class="repo-title">${escapeHTML(b.Title || b.title || b.URL || b.url)}</p>
					<p class="repo-desc">${escapeHTML(b.URL || b.url)}</p>
				</div>
				<button class="btn-secondary bookmark-remove-btn" data-id="${escapeHTML(b.ID || b.id)}">Remove</button>
			</li>
		`).join("");

		list.querySelectorAll(".bookmark-remove-btn").forEach(btn => {
			btn.addEventListener("click", async () => {
				try {
					await apiFetch("/api/bookmarks/remove", { method: "POST", body: JSON.stringify({ id: btn.dataset.id }) });
					refreshBookmarks();
				} catch (e) {}
			});
		});
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load bookmarks</li>`;
	}
}

document.getElementById("bookmark-add-form").addEventListener("submit", async (e) => {
	e.preventDefault();
	const url = document.getElementById("bookmark-url-input").value;
	const title = document.getElementById("bookmark-title-input").value;
	try {
		await apiFetch("/api/bookmarks/add", { method: "POST", body: JSON.stringify({ url, title }) });
		document.getElementById("bookmark-url-input").value = "";
		document.getElementById("bookmark-title-input").value = "";
		refreshBookmarks();
	} catch (e) {}
});

// ===== Enterprise (license, mesh, analytics) =====

async function refreshLicenseStatus() {
	const el = document.getElementById("license-status");
	try {
		const resp = await fetch("/api/license/status").then(r => r.json());
		if (resp.active) {
			el.textContent = `${resp.license.org_name} — ${resp.license.tier} (expires ${new Date(resp.license.expires_at).toLocaleDateString()})`;
		} else {
			el.textContent = "Free tier (no enterprise license active)";
		}
	} catch (e) {
		el.textContent = "Failed to load license status";
	}
}

async function refreshMeshList() {
	const list = document.getElementById("mesh-list");
	try {
		const meshes = await apiFetch("/api/enterprise/mesh");
		if (!meshes || meshes.length === 0) {
			list.innerHTML = `<li class="empty-row">No meshes configured</li>`;
			return;
		}
		list.innerHTML = meshes.map(m => `
			<li class="repo-item">
				<div class="repo-main">
					<p class="repo-title">${escapeHTML(m.Name || m.name)}</p>
					<p class="repo-desc">${escapeHTML(m.OrgName || m.org_name)} — ${(m.MemberIDs || m.member_ids || []).length} members</p>
				</div>
			</li>
		`).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Requires enterprise license</li>`;
	}
}

async function refreshAnalytics() {
	const list = document.getElementById("analytics-list");
	try {
		const snapshots = await apiFetch("/api/enterprise/analytics");
		if (!snapshots || snapshots.length === 0) {
			list.innerHTML = `<li class="empty-row">No analytics snapshots yet</li>`;
			return;
		}
		list.innerHTML = snapshots.slice(-10).reverse().map(s => `
			<li class="repo-item">
				<div class="repo-main">
					<p class="repo-title">${new Date(s.Timestamp || s.timestamp).toLocaleString()}</p>
					<p class="repo-desc">peers: ${s.PeerCount ?? s.peer_count} · cache: ${s.CacheEntries ?? s.cache_entries} entries · served: ${formatBytes(s.BytesServed ?? s.bytes_served)}</p>
				</div>
			</li>
		`).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Requires enterprise license</li>`;
	}
}

document.getElementById("refresh-achievements").addEventListener("click", refreshAchievements);
document.getElementById("refresh-quests").addEventListener("click", refreshQuests);
document.getElementById("refresh-bookmarks").addEventListener("click", refreshBookmarks);
document.getElementById("refresh-analytics").addEventListener("click", refreshAnalytics);

// ===== Discover =====

async function renderDiscoverResults(peers) {
	const list = document.getElementById("discover-list");
	if (!peers || peers.length === 0) {
		list.innerHTML = `<li class="empty-row">No peers found</li>`;
		return;
	}
	list.innerHTML = peers.map(p => `
		<li class="repo-item">
			<div class="repo-main">
				<p class="repo-title">${escapeHTML(p.Name || p.name || "unnamed")}</p>
				<p class="repo-desc"><code>${escapeHTML(p.Address || p.address || "")}</code></p>
			</div>
		</li>
	`).join("");
}

document.getElementById("discover-lan-btn").addEventListener("click", async () => {
	const list = document.getElementById("discover-list");
	list.innerHTML = `<li class="empty-row">Scanning...</li>`;
	try {
		const peers = await apiFetch("/api/discover/lan");
		renderDiscoverResults(peers);
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to scan LAN</li>`;
	}
});

document.getElementById("discover-org-btn").addEventListener("click", async () => {
	const orgID = document.getElementById("discover-org-input").value.trim();
	if (!orgID) return;
	const list = document.getElementById("discover-list");
	list.innerHTML = `<li class="empty-row">Scanning...</li>`;
	try {
		const peers = await apiFetch(`/api/discover/org?org=${encodeURIComponent(orgID)}`);
		renderDiscoverResults(peers);
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to scan org (check org ID)</li>`;
	}
});

// ===== Community Events =====

async function refreshEvents() {
	const list = document.getElementById("events-list");
	try {
		const events = await apiFetch("/api/community/events");
		document.getElementById("events-count").textContent = `${events?.length ?? 0} events`;

		if (!events || events.length === 0) {
			list.innerHTML = `<li class="empty-row">No events yet</li>`;
			return;
		}

		const now = new Date();
		list.innerHTML = events.map(e => {
			const start = new Date(e.StartTime || e.start_time);
			const end = new Date(e.EndTime || e.end_time);
			const isActive = now >= start && now <= end;
			const participantCount = (e.Participants || e.participants || []).length;

			return `
				<li class="repo-item">
					<div class="repo-main">
						<p class="repo-title">${escapeHTML(e.Title || e.title)}</p>
						<p class="repo-desc">${escapeHTML(e.Description || e.description || "")}</p>
						<div class="repo-meta">
							<span>${start.toLocaleString()} — ${end.toLocaleString()}</span>
							<span>${participantCount} participants</span>
						</div>
					</div>
					<span class="pill ${isActive ? "pill-public" : "pill-internal"}">${isActive ? "active" : "scheduled"}</span>
				</li>
			`;
		}).join("");
	} catch (e) {
		list.innerHTML = `<li class="empty-row">Failed to load events</li>`;
	}
}

document.getElementById("event-create-form").addEventListener("submit", async (e) => {
	e.preventDefault();
	const title = document.getElementById("event-title-input").value;
	const start = document.getElementById("event-start-input").value;
	const end = document.getElementById("event-end-input").value;
	try {
		await apiFetch("/api/community/events/create", {
			method: "POST",
			body: JSON.stringify({
				title,
				description: "",
				start_time: new Date(start).toISOString(),
				end_time: new Date(end).toISOString(),
			}),
		});
		document.getElementById("event-title-input").value = "";
		refreshEvents();
	} catch (e) {}
});

document.getElementById("refresh-events").addEventListener("click", refreshEvents);

// ===== boot =====

(async function boot() {
	if (getToken()) {
		try {
			const user = await apiFetch("/api/auth/status");
			await enterApp(user);
			setInterval(refreshAll, REFRESH_INTERVAL);
			return;
		} catch {
			// token invalid/expired, fall through to login
		}
	}
	showLogin();
})();
