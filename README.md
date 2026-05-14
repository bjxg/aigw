<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" alt="License">
</p>

<h1 align="center">🔀 aigw</h1>

<p align="center">
  <strong>A lightweight AI Gateway — unified API entry and management platform.</strong>
</p>

<p align="center">
  English | <a href="README_CN.md">中文</a>
</p>

---

## ⚡ What is aigw?

> **✨ Lightweight downstream fork of [CliRelay](https://github.com/thinkeridea/CliRelay)** — keeps the production-grade management layer while offloading AI proxying and protocol translation to the upstream.

aigw is a lightweight **AI Gateway** that consolidates upstream AI services into a manageable API layer. It provides unified endpoints, usage analytics, request logging, API Key & permission management, **user management**, channel group routing, quota control, model pricing, and a `/manage` web control panel. AI proxy forwarding and protocol translation are handled by CliRelay (upstream [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)) as the upstream; aigw itself does not perform protocol translation or OAuth authentication.

```
┌───────────────────────┐         ┌──────────────┐         ┌────────────────────┐
│   AI Coding Tools     │         │              │         │  Upstream Providers │
│                       │         │              │ ──────▶ │  Google Gemini      │
│  Claude Code          │ ──────▶ │   aigw       │ ──────▶ │  OpenAI / Codex    │
│  Gemini CLI           │         │   :8217      │ ──────▶ │  Anthropic Claude  │
│  OpenAI Codex         │         │              │ ──────▶ │  Vertex / OpenAI   │
│  Any OAI-compatible   │         └──────────────┘         │  ...                │
└───────────────────────┘                                  └────────────────────┘
```

## ✨ Key Features

| Category | Highlights |
|:---------|:-----------|
| **🔌 AI Gateway** | Unified endpoint, smart load balancing, group & path routing, auto failover, OpenAI-compatible |
| **📊 Logging & Monitoring** | Full request capture to SQLite/PostgreSQL, analytics dashboards, health scores, WebSocket live stats |
| **🔐 Access Control** | API Key CRUD, **user management**, per-key quotas & rate limits, permission profiles, key masking |
| **🔗 Channel Management** | Multi-tab provider config, reusable proxy pool, latency tracking, model exclusions, OpenRouter sync |
| **🛠️ Management Panel** | Visual `/manage` UI, i18n (Chinese/English), dark mode, YAML editor, CC Switch import |
| **🗄️ Persistence** | SQLite (default) / PostgreSQL (GORM), optional Redis backup, pluggable config backends (Git/S3) |

> **Note:** aigw strips the OAuth, file-auth, and protocol-translation modules from the original CliRelay (upstream CLIProxyAPI). If you need those capabilities, connect CliRelay as an independent upstream.

## 🏗️ Supported Providers

| Provider | Auth | Notes |
|:---------|:-----|:------|
| Google Gemini | API Key | Gemini style flows |
| Anthropic Claude | API Key | Claude-compatible clients |
| OpenAI Codex | API Key | Includes Responses and WebSocket bridging |
| Vertex-compatible | API Key | Custom base URL, headers, aliases |
| OpenAI-compatible | API Key | OpenRouter, Grok, custom providers |
| Amp integration | API Key + mappings | Direct fallback or mapped local routing |

## 🚀 Quick Start

### Docker Compose

```bash
git clone https://github.com/bjxg/aigw.git
cd aigw
cp config.example.yaml config.yaml
docker compose up -d
```

Edit `config.yaml` to add your API keys, then restart:

```bash
docker compose restart aigw
```

After startup:

- API endpoint: `http://localhost:8217`
- Web panel: `http://localhost:8217/manage`
- Logs: `docker compose logs -f aigw`

By default, client API routes require an API key. To run without keys, set `allow-unauthenticated: true` (not recommended for production).

### Point Your Tools

Set your AI tool's API base to `http://localhost:8217` and start coding!

**Example: OpenAI Codex (`~/.codex/config.toml`)**
```toml
[model_providers.tabcode]
name = "openai"
base_url = "http://localhost:8217/v1"
requires_openai_auth = true
```

> 📖 **Full docs →** [help.router-for.me](https://help.router-for.me/)

## 🖥️ Management Panel

When enabled, open `http://localhost:8217/manage`.

- The server can host bundled SPA assets or auto-fetch panel assets at runtime.
- Panel source is maintained separately at [bjxg/aigw-panel](https://github.com/bjxg/aigw-panel).
- Customize the source via `remote-management.panel-github-repository`.

## 📐 Architecture

```text
aigw/
├── cmd/server/               # Binary entry point
├── internal/api/             # HTTP server, management routes, middleware
├── internal/config/          # Config parsing, defaults, migrations
├── internal/store/           # Local, Git, PostgreSQL, object-store persistence
├── internal/usage/           # SQLite / PostgreSQL DB, analytics, user management
├── internal/managementasset/ # /manage panel hosting and asset sync
├── sdk/                      # Reusable Go SDK, handlers, executors
├── auths/                    # Local credential storage
├── examples/                 # SDK / custom provider examples
└── docker-compose.yml        # Container deployment entry
```

## 🤝 Contributing

This project was born from internal needs, and due to limited bandwidth it is **not actively maintained**. If you need changes, feel free to **fork** the repository and modify it yourself.

## 📜 License

This project is licensed under the **MIT License** — see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgements

This project is directly built upon the excellent open-source **[CliRelay](https://github.com/thinkeridea/CliRelay)** project. CliRelay itself is derived from the upstream **[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)** project. We express our deepest gratitude to both **CliRelay** and **CLIProxyAPI**, as well as all their contributors!

By offloading OAuth and protocol-translation responsibilities back to the upstream, we could focus on lightweight Gateway management features (user management, API Key tracking, request logging, and real-time monitoring) and rebuild an entirely new frontend dashboard from scratch.

A huge salute to the spirit of open source! ❤️
