package usage

import (
	"database/sql"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	log "github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// initGORM initializes the GORM instance on top of an existing *sql.DB.
// This enables the GORM code paths in the usage package while keeping
// the raw *sql.DB available for tables that haven't been migrated yet.
func initGORM(sqlDB *sql.DB, dbPath string) error {
	dialector := sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dbPath,
		Conn:       sqlDB,
	}

	gormConfig := &gorm.Config{
		SkipDefaultTransaction: true,
		// Disable PrepareStmt because the codebase mixes raw *sql.DB operations
		// with GORM, and prepared statement caching can cause stale reads.
		PrepareStmt: false,
	}

	gormDB, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return fmt.Errorf("usage: gorm open: %w", err)
	}

	// Set the global GORM instance in the db package
	db.SetGORM(gormDB)
	log.Infof("usage: GORM initialized on top of existing SQLite connection")
	return nil
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
