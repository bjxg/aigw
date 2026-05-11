package usage

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
)

func normalizeRoutingConfig(input config.RoutingConfig) config.RoutingConfig {
	holder := &config.Config{Routing: input}
	holder.SanitizeRouting()
	return holder.Routing
}

func routingConfigMeaningful(cfg config.RoutingConfig) bool {
	return cfg.Strategy != "" || !cfg.IncludeDefaultGroup || len(cfg.ChannelGroups) > 0 || len(cfg.PathRoutes) > 0
}

func ApplyStoredRoutingConfig(cfg *config.Config) bool {
	if cfg == nil || !ConfigStoreAvailable() {
		return false
	}
	stored := GetRoutingConfig()
	if stored == nil {
		return false
	}
	cfg.Routing = normalizeRoutingConfig(*stored)
	return true
}

func MigrateRoutingConfigFromConfig(cfg *config.Config, configFilePath string) bool {
	if cfg == nil || !ConfigStoreAvailable() {
		return false
	}
	if GetRoutingConfig() != nil {
		cleanRoutingConfigFromYAML(configFilePath)
		return false
	}
	if !routingConfigMeaningful(cfg.Routing) {
		return false
	}
	if err := UpsertRoutingConfig(cfg.Routing); err != nil {
		log.Errorf("usage: migrate routing config: %v", err)
		return false
	}
	if strings.TrimSpace(configFilePath) != "" {
		if backupConfigForMigration(configFilePath, routingMigrationBackupSuffix) {
			cleanRoutingConfigFromYAML(configFilePath)
		}
	}
	return true
}

func GetRoutingConfig() *config.RoutingConfig {
	return GormGetRoutingConfig()
}

func UpsertRoutingConfig(cfg config.RoutingConfig) error {
	return GormUpsertRoutingConfig(cfg)
}
