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
func InitDB(driver, dsn string, storageCfg config.RequestLogStorageConfig, loc *time.Location) error {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()

	if getGormDB() != nil {
		return nil // already initialised
	}

	if loc == nil {
		loc = time.Local
	}
	usageLoc = loc
	usageDBPath = dsn
	_ = storageCfg

	log.Debugf("usage: opening database driver=%s url=%s", driver, dsn)

	gormDB, err := db.Open(driver, dsn, 0, 0)
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

	// 旧正文列只有在全部为空时才清理，失败保留原表并允许服务继续启动。
	if err := dropEmptyLegacyLogContentColumns(gormDB); err != nil {
		log.Warnf("usage: legacy request log columns retained: %v", err)
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

	log.Infof("usage: database initialised driver=%s url=%s", driver, dsn)
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

// 使用原生 DROP COLUMN，避免 SQLite GORM 删列重建表破坏外键或索引。
// 非空值（包括纯空白）禁止删除；两个列在同一事务中检查和删除。
func dropEmptyLegacyLogContentColumns(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		switch tx.Dialector.Name() {
		case "postgres":
			// 阻止检查和删列之间有其他实例写入，同时串行化并发升级。
			if err := tx.Exec("LOCK TABLE request_logs IN ACCESS EXCLUSIVE MODE").Error; err != nil {
				return err
			}
		case "sqlite":
		default:
			return fmt.Errorf("unsupported legacy column migration driver: %s", tx.Dialector.Name())
		}
		columns := make([]string, 0, 2)
		for _, column := range []string{"input_content", "output_content"} {
			if !tx.Migrator().HasColumn("request_logs", column) {
				continue
			}
			var count int64
			if err := tx.Table("request_logs").Where(column + " IS NOT NULL AND LENGTH(" + column + ") > 0").Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("request_logs.%s contains legacy content; migrate it before removing columns", column)
			}
			columns = append(columns, column)
		}
		for _, column := range columns {
			// 列名来自上面的固定白名单，不接受外部输入。
			if err := tx.Exec("ALTER TABLE request_logs DROP COLUMN " + column).Error; err != nil {
				return fmt.Errorf("drop legacy column %s: %w", column, err)
			}
		}
		return nil
	})
}
