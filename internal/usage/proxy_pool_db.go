package usage

import (
	"strings"
	"unicode"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

// ProxyPoolStoreAvailable reports whether the database store is ready for proxy-pool operations.
func ProxyPoolStoreAvailable() bool {
	return getGormDB() != nil
}

// ListProxyPool retrieves all reusable proxies.
func ListProxyPool() []config.ProxyPoolEntry {
	return GormListProxyPool()
}

// GetProxyPoolEntry retrieves one reusable proxy by ID.
func GetProxyPoolEntry(id string) *config.ProxyPoolEntry {
	return GormGetProxyPoolEntry(id)
}

// ReplaceProxyPool atomically replaces all proxy entries after normalization.
func ReplaceProxyPool(entries []config.ProxyPoolEntry) error {
	return GormReplaceProxyPool(entries)
}

// ApplyStoredProxyPool overlays the DB-backed proxy pool onto the runtime config.
func ApplyStoredProxyPool(cfg *config.Config) bool {
	if cfg == nil || !ProxyPoolStoreAvailable() {
		return false
	}
	cfg.ProxyPool = ListProxyPool()
	return true
}

// MigrateProxyPoolFromConfig moves legacy YAML proxy-pool entries into the database.
func MigrateProxyPoolFromConfig(cfg *config.Config, configFilePath string) int {
	if cfg == nil || !ProxyPoolStoreAvailable() {
		return 0
	}
	if len(ListProxyPool()) > 0 {
		cfg.ProxyPool = nil
		cleanProxyPoolFromYAML(configFilePath)
		return 0
	}
	if len(cfg.ProxyPool) == 0 {
		return 0
	}

	normalized := config.NormalizeProxyPool(cfg.ProxyPool)
	if len(normalized) == 0 {
		cfg.ProxyPool = nil
		if backupConfigForMigration(configFilePath, proxyPoolMigrationBackupSuffix) {
			cleanProxyPoolFromYAML(configFilePath)
		}
		return 0
	}

	if err := ReplaceProxyPool(normalized); err != nil {
		log.Errorf("usage: migrate proxy_pool: %v", err)
		return 0
	}
	cfg.ProxyPool = nil
	if configFilePath != "" {
		if backupConfigForMigration(configFilePath, proxyPoolMigrationBackupSuffix) {
			cleanProxyPoolFromYAML(configFilePath)
		}
	}
	log.Infof("usage: migrated %d proxy_pool entries from config to database", len(normalized))
	return len(normalized)
}

func normalizeProxyPoolEntryID(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
