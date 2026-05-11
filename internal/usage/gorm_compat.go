package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// InitDB opens (or creates) the database and initializes GORM.
// It uses db.Open to create a GORM connection with the appropriate driver,
// then runs AutoMigrate and seeds initial data.
func InitDB(dbPath string, storageCfg config.RequestLogStorageConfig, loc *time.Location) error {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()

	if getGormDB() != nil {
		return nil // already initialised
	}

	if loc == nil {
		loc = time.Local
	}
	usageLoc = loc
	usageDBPath = dbPath
	requestLogStorage = normalizeRequestLogStorageConfig(storageCfg)

	log.Debugf("usage: opening database at %s", dbPath)

	gormDB, err := db.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("usage: open database: %w", err)
	}

	// Verify connectivity with a timeout to avoid hanging on WAL recovery
	log.Debugf("usage: pinging database to verify connectivity")
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("usage: get underlying db: %w", err)
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pingCancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("usage: ping database: %w", err)
	}

	// Run GORM AutoMigrate for all models
	log.Debugf("usage: running GORM auto-migration")
	if err := runGORMAutoMigrate(); err != nil {
		log.Warnf("usage: GORM auto-migration failed: %v", err)
	}

	// Seed default data and load caches
	log.Debugf("usage: merging legacy pricing into model_configs")
	mergeLegacyPricingIntoModelConfigs()
	log.Debugf("usage: seeding default model config rows")
	gormSeedDefaultModelConfigRows()
	log.Debugf("usage: reloading model config cache")
	gormReloadModelConfigCache()
	log.Debugf("usage: reloading model owner preset cache")
	gormReloadModelOwnerPresetCache()
	log.Debugf("usage: reloading pricing cache")
	gormReloadPricingCache()
	log.Debugf("usage: backfilling API key names")
	GormBackfillAPIKeyNames()
	log.Debugf("usage: ensuring openrouter model sync state row")
	GormEnsureOpenRouterModelSyncStateRow()

	log.Infof("usage: database initialised at %s", dbPath)
	return nil
}

// CloseDB closes the database gracefully.
func CloseDB() {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()

	stopRequestLogMaintenance()

	if err := db.Close(); err != nil {
		log.Warnf("usage: close database: %v", err)
	}
	usageLoc = nil
	usageDBPath = ""
	log.Info("usage: database closed")
}

// runGORMAutoMigrate runs GORM AutoMigrate for models that correspond to
// tables not yet present in the database. For existing tables, it only
// adds missing columns using Migrator().AddColumn() to avoid GORM's
// destructive table-rebuild on tables with constraints it doesn't understand
// (e.g. FOREIGN KEY in request_log_content).
func runGORMAutoMigrate() error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: GORM not initialized")
	}

	m := gormDB.Migrator()

	for _, model := range AllModels() {
		tableName, err := getModelTableName(gormDB, model)
		if err != nil {
			log.Warnf("usage: GORM auto-migrate: cannot get table name: %v", err)
			continue
		}

		if m.HasTable(model) {
			// Table already exists — only add missing columns
			stmt := &gorm.Statement{DB: gormDB}
			if err := stmt.Parse(model); err != nil {
				log.Warnf("usage: GORM auto-migrate: parse model %s: %v", tableName, err)
				continue
			}
			for _, field := range stmt.Schema.Fields {
				if !m.HasColumn(model, field.DBName) {
					if err := m.AddColumn(model, field.DBName); err != nil {
						log.Warnf("usage: GORM add column %s.%s: %v", tableName, field.DBName, err)
					} else {
						log.Infof("usage: GORM added column %s.%s", tableName, field.DBName)
					}
				}
			}
		} else {
			// New table — safe to AutoMigrate
			if err := gormDB.AutoMigrate(model); err != nil {
				log.Warnf("usage: GORM auto-migrate %s: %v", tableName, err)
			} else {
				log.Infof("usage: GORM created table %s", tableName)
			}
		}
	}

	return nil
}

// getModelTableName returns the GORM table name for a model.
func getModelTableName(d *gorm.DB, model interface{}) (string, error) {
	stmt := &gorm.Statement{DB: d}
	if err := stmt.Parse(model); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}
