// FLOW Service Worker — intercepts fetch requests from the page that
// registers this worker, checks the local FLOW daemon's cache first
// (via its HTTP API on localhost), and falls back to the real network
// if the daemon doesn't have it. This is what "Service Worker Mode"
// means for FLOW: browser-side integration with the P2P cache daemon,
// not a browser cache API rename.

const FLOW_API_BASE = "http://localhost:7677";
let flowAuthToken = null;

// The page that registers this worker must postMessage its auth token
// in, since service workers can't read localStorage directly.
self.addEventListener("message", (event) => {
	if (event.data && event.data.type === "FLOW_AUTH_TOKEN") {
		flowAuthToken = event.data.token;
	}
});

self.addEventListener("install", (event) => {
	self.skipWaiting();
});

self.addEventListener("activate", (event) => {
	event.waitUntil(self.clients.claim());
});

// Hash a URL the same way the daemon does (SHA-256 of "url|level"),
// using the Web Crypto API so this stays dependency-free.
async function flowCacheKey(url, level) {
	const input = `${url}|${level}`;
	const encoder = new TextEncoder();
	const data = encoder.encode(input);
	const hashBuffer = await crypto.subtle.digest("SHA-256", data);
	const hashArray = Array.from(new Uint8Array(hashBuffer));
	return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function tryFlowCache(request) {
	if (!flowAuthToken) return null;
	if (request.method !== "GET") return null;

	try {
		const level = 1; // matches Fetcher's default compression level
		const hash = await flowCacheKey(request.url, level);

		const resp = await fetch(`${FLOW_API_BASE}/api/cache/read?hash=${hash}`, {
			headers: { Authorization: `Bearer ${flowAuthToken}` },
		});

		if (!resp.ok) return null;

		const contentType = resp.headers.get("Content-Type") || "application/octet-stream";
		const body = await resp.arrayBuffer();
		return new Response(body, {
			status: 200,
			headers: { "Content-Type": contentType, "X-Served-By": "flow-p2p-cache" },
		});
	} catch (e) {
		return null; // daemon unreachable, silently fall through to network
	}
}

self.addEventListener("fetch", (event) => {
	// only intercept http/https GET requests, leave everything else
	// (extensions, chrome-extension://, websocket upgrades) untouched
	const url = new URL(event.request.url);
	if (!url.protocol.startsWith("http")) return;

	event.respondWith(
		(async () => {
			const cached = await tryFlowCache(event.request);
			if (cached) return cached;
			return fetch(event.request);
		})()
	);
});
