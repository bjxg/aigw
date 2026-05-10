# CHANGELOG

## Unreleased

### API Key ID migration (breaking change)

- **Backend**: Migrated `api_keys` table primary key from string `key` to auto-increment integer `id`
- **Backend**: Changed `request_logs` table's `api_key` field to `api_key_id` integer foreign key pointing to `api_keys.id`
- **Backend**: Updated `FilterOptions` struct to use `APIKeyFilterItem` array instead of string array for API key filtering
- **Backend**: Optimized system request detection logic - now directly checks `api_key_id = 0` instead of string matching
- **Backend**: Refactored filter options retrieval - queries `api_keys` table directly instead of distinct values from `request_logs`
- **Backend**: Added `api_key` string to `api_key_id` conversion in public log query API for backward compatibility
- **Frontend**: Updated `UsageLogItem` type definition to include `api_key_id` field
- **Frontend**: Changed `FilterOptions` to use `APIKeyFilterItem` array for API key filter options
- **Frontend**: Replaced text input filter with `SearchableSelect` component for API key filtering in request logs
- **Frontend**: Updated `isSystemRequestLogKey` helper to check `api_key_id === 0`
- **Frontend**: Fixed TypeScript type mismatches in `ApiKeysPage`, `ProvidersPage`, and `useMonitorDashboardState`
- **Tests**: Updated all test files to use integer `api_key_id` values instead of string `api_key` values
- **Tests**: Fixed native SQL statements in `usage_db_test.go` to match new schema

### Codex auth metadata and startup registration

- persisted Codex `plan_type` into runtime auth metadata during both CLI/device login and management OAuth login flows
- preserved Codex `account_id` explicitly in runtime auth metadata for downstream request handling
- backfilled Codex `plan_type` from legacy `id_token` metadata when older auth files do not store it explicitly
- registered loaded auths during service startup so executors and model visibility are available immediately after boot
- preserved Codex free-tier model routing without forcing tier-based model downgrades or synthetic excluded-model lists

### Verification and tests

- added auth metadata coverage test for Codex plan/account persistence
- added watcher synthesis coverage for Codex `plan_type` backfill and free-tier pass-through behavior
- added service startup registration coverage for loaded auth records

### Notes

- this change is aimed at Codex OAuth / ChatGPT-account free-tier behavior, not standard OpenAI API-key billing flows
