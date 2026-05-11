// Package cmd provides command-line interface functionality for the CLI Proxy API server.
// It includes authentication flows for various AI service providers, service startup,
// and other command-line operations.
package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/api"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/middleware"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy"
	log "github.com/sirupsen/logrus"
)

// StartService builds and runs the proxy service using the exported SDK.
// It creates a new proxy service instance, sets up signal handling for graceful shutdown,
// and starts the service with the provided configuration.
//
// Parameters:
//   - cfg: The application configuration
//   - configPath: The path to the configuration file
//   - localPassword: Optional password accepted for local management requests
func StartService(cfg *config.Config, configPath string, localPassword string) {
	loc := config.ApplyTimeZone(cfg.Timezone)
	dataDir := filepath.Join(filepath.Dir(configPath), "data")
	_ = os.MkdirAll(dataDir, 0755)

	// Build URL: use cfg.Database.URL if set, otherwise fall back to dataDir/usage.db
	url := cfg.Database.URL
	if url == "" {
		url = filepath.Join(dataDir, "usage.db")
	}
	dbDriver := cfg.Database.Driver
	if dbDriver == "" {
		dbDriver = "sqlite"
	}

	// Migrate legacy usage.db from config directory to data/ subdirectory.
	// Only applies when using SQLite and the effective URL points to data/usage.db.
	if dbDriver == "sqlite" && url == filepath.Join(dataDir, "usage.db") {
		legacyPath := filepath.Join(filepath.Dir(configPath), "usage.db")
		if _, err := os.Stat(legacyPath); err == nil {
			if _, err := os.Stat(url); os.IsNotExist(err) {
				if err := os.Rename(legacyPath, url); err != nil {
					log.Warnf("usage: failed to migrate %s → %s: %v", legacyPath, url, err)
				} else {
					log.Infof("usage: migrated database from %s → %s", legacyPath, url)
					for _, suffix := range []string{"-wal", "-shm"} {
						if err := os.Rename(legacyPath+suffix, url+suffix); err != nil && !os.IsNotExist(err) {
							log.Warnf("usage: failed to migrate %s: %v", legacyPath+suffix, err)
						}
					}
				}
			}
		}
	}

	if err := usage.InitDB(dbDriver, url, cfg.RequestLogStorage, loc); err != nil {
		log.Errorf("usage: failed to initialize database: %v", err)
	}
	usage.MigrateAPIKeysFromConfig(cfg, configPath)
	usage.MigrateAPIKeyPermissionProfilesFromYAML(configPath)
	usage.MigrateRoutingConfigFromConfig(cfg, configPath)
	usage.ApplyStoredRoutingConfig(cfg)
	usage.MigrateProxyPoolFromConfig(cfg, configPath)
	usage.ApplyStoredProxyPool(cfg)
	usage.MigrateRuntimeSettingsFromConfig(cfg, configPath)
	usage.ApplyStoredRuntimeSettings(cfg)
	middleware.InitQuotaUsageFuncs(usage.CountTodayByKey, usage.CountTotalByKey, usage.QueryTotalCostByKey)
	usage.SetTokenUsageCallback(middleware.RecordTokenUsage)
	usage.InitRedis(cfg.Redis)
	defer usage.StopRedis()

	builder := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(configPath).
		WithLocalManagementPassword(localPassword)

	ctxSignal, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	runCtx := ctxSignal
	if localPassword != "" {
		var keepAliveCancel context.CancelFunc
		runCtx, keepAliveCancel = context.WithCancel(ctxSignal)
		builder = builder.WithServerOptions(api.WithKeepAliveEndpoint(10*time.Second, func() {
			log.Warn("keep-alive endpoint idle for 10s, shutting down")
			keepAliveCancel()
		}))
	}

	service, err := builder.Build()
	if err != nil {
		log.Errorf("failed to build proxy service: %v", err)
		return
	}

	err = service.Run(runCtx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("proxy service exited with error: %v", err)
	}
}

// StartServiceBackground starts the proxy service in a background goroutine
// and returns a cancel function for shutdown and a done channel.
func StartServiceBackground(cfg *config.Config, configPath string, localPassword string) (cancel func(), done <-chan struct{}) {
	loc := config.ApplyTimeZone(cfg.Timezone)
	dataDir := filepath.Join(filepath.Dir(configPath), "data")
	_ = os.MkdirAll(dataDir, 0755)

	url := cfg.Database.URL
	if url == "" {
		url = filepath.Join(dataDir, "usage.db")
	}
	dbDriver := cfg.Database.Driver
	if dbDriver == "" {
		dbDriver = "sqlite"
	}

	if dbDriver == "sqlite" && url == filepath.Join(dataDir, "usage.db") {
		legacyPath := filepath.Join(filepath.Dir(configPath), "usage.db")
		if _, err := os.Stat(legacyPath); err == nil {
			if _, err := os.Stat(url); os.IsNotExist(err) {
				if err := os.Rename(legacyPath, url); err != nil {
					log.Warnf("usage: failed to migrate %s → %s: %v", legacyPath, url, err)
				} else {
					log.Infof("usage: migrated database from %s → %s", legacyPath, url)
					for _, suffix := range []string{"-wal", "-shm"} {
						if err := os.Rename(legacyPath+suffix, url+suffix); err != nil && !os.IsNotExist(err) {
							log.Warnf("usage: failed to migrate %s: %v", legacyPath+suffix, err)
						}
					}
				}
			}
		}
	}

	if err := usage.InitDB(dbDriver, url, cfg.RequestLogStorage, loc); err != nil {
		log.Errorf("usage: failed to initialize database: %v", err)
	}
	usage.MigrateAPIKeysFromConfig(cfg, configPath)
	usage.MigrateAPIKeyPermissionProfilesFromYAML(configPath)
	usage.MigrateRoutingConfigFromConfig(cfg, configPath)
	usage.ApplyStoredRoutingConfig(cfg)
	usage.MigrateProxyPoolFromConfig(cfg, configPath)
	usage.ApplyStoredProxyPool(cfg)
	usage.MigrateRuntimeSettingsFromConfig(cfg, configPath)
	usage.ApplyStoredRuntimeSettings(cfg)
	middleware.InitQuotaUsageFuncs(usage.CountTodayByKey, usage.CountTotalByKey, usage.QueryTotalCostByKey)
	usage.SetTokenUsageCallback(middleware.RecordTokenUsage)
	usage.InitRedis(cfg.Redis)

	builder := cliproxy.NewBuilder().
		WithConfig(cfg).
		WithConfigPath(configPath).
		WithLocalManagementPassword(localPassword)

	ctx, cancelFn := context.WithCancel(context.Background())
	doneCh := make(chan struct{})

	service, err := builder.Build()
	if err != nil {
		log.Errorf("failed to build proxy service: %v", err)
		close(doneCh)
		return cancelFn, doneCh
	}

	go func() {
		defer close(doneCh)
		defer usage.StopRedis()
		if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("proxy service exited with error: %v", err)
		}
	}()

	return cancelFn, doneCh
}

// WaitForCloudDeploy waits indefinitely for shutdown signals in cloud deploy mode
// when no configuration file is available.
func WaitForCloudDeploy() {
	// Clarify that we are intentionally idle for configuration and not running the API server.
	log.Info("Cloud deploy mode: No config found; standing by for configuration. API server is not started. Press Ctrl+C to exit.")

	ctxSignal, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Block until shutdown signal is received
	<-ctxSignal.Done()
	log.Info("Cloud deploy mode: Shutdown signal received; exiting")
}
