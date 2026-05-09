package db

import (
	"fmt"

	"gorm.io/gorm"
	log "github.com/sirupsen/logrus"
)

// AutoMigrate runs GORM auto-migration for all given models.
// It creates tables and missing columns, but does NOT drop existing columns or data.
func AutoMigrate(d *gorm.DB, models ...interface{}) error {
	if d == nil {
		return fmt.Errorf("db: cannot auto-migrate: database not initialised")
	}
	if err := d.AutoMigrate(models...); err != nil {
		return fmt.Errorf("db: auto-migrate: %w", err)
	}
	log.Infof("db: auto-migration completed for %d model(s)", len(models))
	return nil
}
