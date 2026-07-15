# FLOW

![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)

[LICENSE](LICENSE)

![Go Version](https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go)

![Platform](https://img.shields.io/badge/platform-linux%20%7C%20android%20(termux)

**Federated Local Offload Web** — a peer-to-peer web caching system, built like BitTorrent but for the web. Peers share locally cached content with each other instead of every request hitting the origin server.

Built in Go, runs as a daemon (`flow`) controlled via a CLI (`flc`), with a full web dashboard. Designed to run on Termux/Android ARM64 as well as standard Linux.

┌──────────┐      WSS       ┌──────────┐      WSS       ┌──────────┐
│  Peer A  │◄──────────────►│  Peer B  │◄──────────────►│  Peer C  │
│ (cache)  │  Kademlia DHT  │ (cache)  │  chunk pull    │ (cache)  │
└──────────┘                └──────────┘                └──────────┘

## Features

- **Real P2P core** — Kademlia DHT (XOR distance, k-buckets, iterative FIND_NODE/FIND_VALUE lookup), chunk-based transfer with checksum verification, reputation-aware peer selection, auto-connect (no manual dialing needed)
- **P2P-first fetching** — checks the network for content (via DHT provider lookup + active manifest requests) before ever hitting the origin server; falls back to HTTP only when no peer has it
- **Quantized compression** — `.flow` format with 5 levels (lossless up to 8x smaller), real HTML minification, CSS tree-shaking, and JPEG re-encoding
- **Security** — TLS 1.3, Ed25519 peer identity, automatic certificate rotation, content blinding (AES-256-GCM keyed by source URL), phishing blacklist, malware scanning (ClamAV `clamd` protocol), rate-limited login
- **Social & gamification** — leaderboard, 22 achievement badges, rotating daily quests, community events, per-peer README pages (Markdown + MDX, XSS-sanitized rendering)
- **Enterprise** — offline license validation (HMAC-signed), mesh networking with priority routing, analytics with CSV export
- **Extras** — Wikipedia offline pre-caching, video pre-rolling (byte-range), bookmark sync across devices (LAN, last-write-wins), multi-device LAN discovery, Smart DNS (DNS-over-HTTPS), browser Service Worker integration, incognito mode, hot-reload config, embedded pprof profiler

## Installation

Requires Go 1.21+.

```bash
git clone https://github.com/Natarizki/flow.git
cd flow
go mod tidy
make build
```
This produces two binaries in bin/: flow (the daemon) and flc (the CLI).

## Install to PATH
**Termux:**
```bash
cp bin/flow $PREFIX/bin/flow
cp bin/flc $PREFIX/bin/flc
```
**Standard Linux:**
```bash
sudo cp bin/flow /usr/local/bin/flow
sudo cp bin/flc /usr/local/bin/flc
```
## Shell completion

**bash:**
```bash
mkdir -p ~/.bash_completion.d
flc completion bash > ~/.bash_completion.d/flc
echo 'source ~/.bash_completion.d/flc' >> ~/.bashrc
source ~/.bashrc
```
zsh:
```bash
mkdir -p ~/.zsh/completions
flc completion zsh > ~/.zsh/completions/_flc
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit' >> ~/.zshrc
source ~/.zshrc
```
fish:
```bash
mkdir -p ~/.config/fish/completions
flc completion fish > ~/.config/fish/completions/flc.fish
```
After installing, verify it's registered:
```bash
which flc
flc <TAB>
```
You should see the full subcommand list (auth, create, compress, cache, prefetch, leaderboard, whois, bookmark, etc.) instead of a plain file listing.

## Quick Start
**Start the daemon:**
```bash
flow
```
**In another terminal, register an account and create a peer:**
```bash
flc auth register --email you@example.com --username you --password yourpassword
flc auth login --email you@example.com --password yourpassword
flc create --peer myfirstpeer --visible public
```
**Compress and cache a file:**
```bash
flc compress myfile.html --level 3
```
Open the dashboard at http://localhost:7677 to see peers, cache stats, achievements, quests, and more.

## CLI Reference
�
Click to expand full command list

Auth:
  flc auth login|register|logout|status|refresh

Peer management:
  flc create|list|show|delete|rename|visibility|lock|unlock <peer>
  flc readme <peer> --file README.md|README.mdx

Content:
  flc push <peer> <file>
  flc pull <peer> <file>
  flc compress|decompress|convert|info <file>

Cache:
  flc cache list|clean|export|import|stats

Discovery & network:
  flc discover --lan|--org <name>
  flc whois <peer>
  flc msg <peer> "<message>"
  flc bandwidth --today|--month

Organization:
  flc tag add|remove|list
  flc org create|join|list

Prefetch:
  flc prefetch train|predict|enable|disable|now

Social:
  flc leaderboard
  flc achievements --peer <peer>
  flc bookmark add|list

Media:
  flc wikipedia-precache --lang en

Shell:
  flc completion bash|zsh|fish

## Architecture

flow/
├── cmd/
│   ├── flow/         # daemon entry point
│   └── flc/          # CLI entry point
├── internal/
│   ├── auth/         # JWT auth, bcrypt, sessions
│   ├── websocket/    # WSS hub, RPC tracker, message protocol
│   ├── p2p/          # Kademlia DHT, chunk transfer, peer/tag/org management, LAN sync
│   ├── cache/        # LRU eviction, disk storage, tar export/import, incognito mode
│   ├── compression/  # quantization engine, levels 0-4
│   ├── fetcher/      # HTTP fetch + P2P-first lookup + compress + cache pipeline
│   ├── prefetch/     # Markov chain predictor, video preroll, Wikipedia precache
│   ├── security/     # TLS, Ed25519 identity, phishing/malware, cert rotation, content blinding
│   ├── api/          # HTTP API, CORS, rate limiting, dashboard serving
│   ├── social/       # leaderboard, achievements, quests, bookmarks, bandwidth tracking
│   ├── enterprise/   # license validation, mesh, analytics
│   ├── network/      # Smart DNS (DNS-over-HTTPS)
│   ├── config/       # hot-reload config watcher
│   └── store/        # BadgerDB persistence layer
├── pkg/utils/        # shared logging, config, errors, hashing
├── web/dashboard/    # HTML/CSS/JS dashboard, README renderer, service worker
└── docs/             # protocol and format documentation

## Configuration

FLOW reads flow.yaml from the working directory:

```yaml
daemon_port: 7676
dashboard_port: 7677
cache_dir: ./flow-cache
cache_max_size: 10737418240   # 10GB
tracker_url: ""
log_level: info
content_blinding_enabled: true
incognito_mode: false
```
Changes to cache_max_size and content_blinding_enabled are hot-reloaded without restarting the daemon.

## Security Notes
- flow-cache/identity.key is your node's private Ed25519 key — never commit or share it.
- Content blinding is on by default: cached content is encrypted at rest with a key derived from its source URL, so whoever holds the disk can't read cached content without knowing the URL.
- Malware scanning requires a running clamd instance and is disabled by default until you've confirmed clamd is reachable at 127.0.0.1:3310.
- Peer READMEs are rendered client-side with DOMPurify sanitization — scripts and event handlers are stripped before rendering.

## Roadmap

- [ ] WiFi Direct bridge app (Kotlin/Android) — deferred until core is fully stable
- [ ] Full CSS selector engine for tree-shaking (currently handles simple/compound selectors)
- [ ] Persistent DHT bucket refresh loop

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Author

Built by [Natarizki](https://github.com/Natarizki).
