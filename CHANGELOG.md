# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [0.2.4] - 2026-10-17

### Added
- **Chart Data Total Users**: The `/v0/management/usage/chart-data` endpoint now returns `total_users` — the number of distinct users who issued requests within the selected time range (deduplicated across API keys, includes failed requests; requests without an associated user are excluded). Honors the existing API key filter and the `days` / date-range parameters (`internal/usage/repo_logs.go`, `internal/api/handlers/management/usage_logs_handler.go`).

## [0.2.3] - 2026-10-16

### Fixed
- **Claude Token Statistics**: Claude usage parsing (`parseClaudeUsage` / `parseClaudeStreamUsage`) now normalizes `input_tokens` to include `cache_read_input_tokens` and `cache_creation_input_tokens`, matching OpenAI usage semantics (where `prompt_tokens` already includes cached tokens). As a result, `request_logs.input_tokens` and `total_tokens` now reflect the full context for Claude protocol requests instead of omitting cached tokens.
- **Startup Error Handling**: `StartService` / `StartServiceBackground` now propagate startup failures as errors instead of only logging them; the server exits with code 1 and a clear `failed to start service` message when the database or proxy service fails to initialize (previously startup could fail silently, e.g. on an unreachable PostgreSQL).
- Note: the Claude token fix also aligns TPM quota accounting, since TPM tracking is driven by `total_tokens`. Historical rows in `request_logs` are not retroactively updated.

### Changed
- **Database Config Sanitization**: `SanitizeDatabase` now accepts `postgresql` as a driver alias for `postgres`, infers the `postgres` driver from `postgres://` / `postgresql://` URL schemes, and no longer silently falls back to SQLite for unknown drivers. Startup fails fast with an explicit error when PostgreSQL is selected without a `database.url` (`internal/config/config.go`, `internal/cmd/run.go`).

## [Unreleased]

### Changed
- **README**: Updated project positioning to reflect the new architecture based on CliRelay.
- **Docs**: Simplified documentation and removed outdated feature descriptions.

## [0.1.0] - 2025-XX-XX

### Overview
This release marks the initial transformation of the project from a full AI CLI proxy into a lightweight **AI Gateway**. The core AI proxying and protocol translation responsibilities have been offloaded to [CliRelay](https://github.com/kittors/CliRelay) (upstream [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)), allowing this project to focus on management, analytics, and access control.

### Removed
- **OAuth Module**: Removed all OAuth authentication flows (Gemini, Claude, Codex, Qwen, etc.) from the core codebase.
- **File Authentication**: Removed the file-based authentication system.
- **Protocol Translation**: Removed the protocol format translation layer.
- **TUI (Terminal UI)**: Removed the terminal-based management interface.
- **Updater Sidecar**: Removed the built-in auto-updater component.

### Added
- **User Management**: Introduced a standalone user system with user CRUD operations and role-based associations.
- **API Key Association**: API Keys can now be linked to specific users for resource ownership and permission control.
- **PostgreSQL Support**: Added GORM-based PostgreSQL backend support alongside the existing SQLite database.

### Enhanced
- **Usage Analytics**: Migrated the usage database layer to GORM, enabling compatibility with PostgreSQL.
- **API Key Management**: Added support for permission profiles to bulk-control available models, channel groups, and quota limits.
- **Management Panel (`/manage`)**: Completely rebuilt the frontend dashboard to align with the new Gateway-focused architecture.

### Architecture
- `internal/auth/` and `internal/tui/` directories removed.
- `internal/usage/` expanded to include user management responsibilities.
- Legacy `internal/store/` config/auth mirror removed.

### Notes
- If OAuth, file authentication, or protocol translation capabilities are required, users should deploy **CliRelay** as an independent upstream and configure `aigw` to route traffic through it.
