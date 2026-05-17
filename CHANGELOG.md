# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
