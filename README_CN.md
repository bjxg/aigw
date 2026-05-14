<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" alt="License">
</p>

<h1 align="center">🔀 aigw</h1>

<p align="center">
  <strong>轻量级 AI Gateway — 统一的 API 入口与管理平台。</strong>
</p>

<p align="center">
  <a href="README.md">English</a> | 中文
</p>

---

## ⚡ aigw 是什么？

> **✨ 基于 [CliRelay](https://github.com/kittors/CliRelay) 的轻量级二次开发版** — 保留生产级管理层，将 AI 代理与协议转换交由上游处理。

aigw 是一个轻量级 **AI Gateway**，专注于把上游 AI 服务整合成可管理的 API 层。它提供统一端点、用量数据分析、请求日志、API Key 与权限管理、**用户管理**、渠道分组路由、配额管控、模型定价、`/manage` Web 控制面板等核心能力。AI 代理转发与协议格式转换由 CliRelay（上游 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)）作为上游完成，aigw 自身不承担协议翻译与 OAuth 认证等职责。

```
┌───────────────────────┐         ┌──────────────┐         ┌────────────────────┐
│   AI 编程工具          │         │              │         │  上游服务商          │
│                       │         │              │ ──────▶ │  Google Gemini      │
│  Claude Code          │ ──────▶ │   aigw       │ ──────▶ │  OpenAI / Codex    │
│  Gemini CLI           │         │   :8217      │ ──────▶ │  Anthropic Claude  │
│  OpenAI Codex         │         │              │ ──────▶ │  Vertex / OpenAI   │
│  任意 OAI 兼容客户端   │         └──────────────┘         │  ...                │
└───────────────────────┘                                  └────────────────────┘
```

## ✨ 核心特性

| 分类 | 亮点 |
|:-----|:-----|
| **🔌 AI Gateway** | 统一端点、智能负载均衡、分组与路径路由、自动故障转移、OpenAI 兼容 |
| **📊 日志与监控** | 完整请求捕获到 SQLite/PostgreSQL、分析仪表盘、健康评分、WebSocket 实时监控 |
| **🔐 权限管控** | API Key CRUD、**用户管理**、单 Key 配额与速率限制、权限配置模板、Key 脱敏 |
| **🔗 渠道管理** | 多标签页服务商配置、可复用代理池、延迟追踪、模型排除、OpenRouter 同步 |
| **🛠️ 管理面板** | 可视化 `/manage` 界面、中英文 i18n、暗色模式、YAML 编辑器、CC Switch 导入 |
| **🗄️ 数据持久化** | SQLite（默认）/ PostgreSQL（GORM）、可选 Redis 备份、可插拔配置后端（Git/S3） |

> **注：** aigw 剥离了原 CliRelay（上游 CLIProxyAPI）中的 OAuth、文件认证与协议翻译模块。若需要这些能力，请将 CliRelay 作为独立上游接入。

## 🏗️ 支持的服务商

| 服务商 | 认证方式 | 说明 |
|:-------|:---------|:-----|
| Google Gemini | API Key | 适配 Gemini 风格链路 |
| Anthropic Claude | API Key | 面向 Claude 兼容客户端 |
| OpenAI Codex | API Key | 包含 Responses 与 WebSocket 桥接 |
| Vertex 兼容端点 | API Key | 支持自定义 base URL、Header、别名 |
| OpenAI 兼容上游 | API Key | OpenRouter、Grok 及自定义 provider |
| Amp 集成 | API Key + 映射 | 直接回退或映射到本地可用模型 |

## 🚀 快速开始

### Docker Compose

```bash
git clone https://github.com/bjxg/aigw.git
cd aigw
cp config.example.yaml config.yaml
docker compose up -d
```

编辑 `config.yaml` 添加你的 API 密钥，然后重启：

```bash
docker compose restart cli-proxy-api
```

启动后常用入口：

- API 地址：`http://localhost:8217`
- Web 面板：`http://localhost:8217/manage`
- 查看日志：`docker compose logs -f cli-proxy-api`

默认情况下，客户端 API 路由需要 API Key；如需在未配置的情况下运行，可设置 `allow-unauthenticated: true`（生产环境不推荐）。

### 配置工具

将 AI 工具的 API 地址设为 `http://localhost:8217`，开始编码！

**示例：OpenAI Codex (`~/.codex/config.toml`)**
```toml
[model_providers.tabcode]
name = "openai"
base_url = "http://localhost:8217/v1"
requires_openai_auth = true
```

> 📖 **完整文档 →** [help.router-for.me](https://help.router-for.me/cn/)

## 🖥️ 管理面板

启用后访问 `http://localhost:8217/manage`。

- 服务端支持托管打包后的 SPA 资源，或在需要时自动拉取面板资源。
- 面板源码独立维护，默认仓库为 [bjxg/aigw-panel](https://github.com/bjxg/aigw-panel)。
- 可通过 `remote-management.panel-github-repository` 自定义面板资源来源。

## 📐 项目结构

```text
aigw/
├── cmd/server/               # 二进制入口
├── internal/api/             # HTTP 服务、管理路由、中间件
├── internal/config/          # 配置解析、默认值、迁移
├── internal/store/           # 本地、Git、PostgreSQL、对象存储持久化
├── internal/usage/           # SQLite / PostgreSQL 数据库、分析聚合、用户管理
├── internal/managementasset/ # /manage 面板托管与资源同步
├── sdk/                      # 可复用 Go SDK、handlers、executors
├── auths/                    # 本地凭据存储
├── examples/                 # SDK / 自定义 provider 示例
└── docker-compose.yml        # 容器部署入口
```

## 🤝 贡献

本项目由内部需求驱动发起，因精力有限，**暂不做持续维护**。如有需要，欢迎 **fork** 代码后自行修改。

## 📜 许可证

本项目采用 **MIT 许可证** — 详见 [LICENSE](LICENSE) 文件。

---

## 🙏 特别鸣谢

本项目直接基于优秀的开源项目 **[CliRelay](https://github.com/kittors/CliRelay)** 进行深度开发。CliRelay 本身又衍生自上游项目 **[router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)**。在此，我们对 **CliRelay**、**CLIProxyAPI** 以及它们的全体贡献者表达最诚挚的感谢！

正是由于上游构建的坚实底座，我们才能剥离复杂的 OAuth 与协议翻译职责，专注于轻量级 Gateway 所需的高级管理功能（如用户管理、API Key 追踪管控、完整的请求日志、实时系统监控），并完全重构了前端管理面板。

饮水思源，向开源精神致敬！❤️
