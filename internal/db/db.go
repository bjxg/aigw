// Package db provides GORM database initialization, multi-driver support,
// and connection pool management for the aigw-server project.
package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "modernc.org/sqlite"

	log "github.com/sirupsen/logrus"
)

var (
	gormDB *gorm.DB
	mu     sync.Mutex
)

// Open creates a GORM database connection based on the driver name and DSN.
// Supported drivers: "sqlite", "postgres".
// For SQLite, it opens the database using modernc.org/sqlite (pure Go, no CGO),
// then wraps it with GORM's SQLite dialector to avoid driver registration conflicts.
// For PostgreSQL, it opens using pgx connection pool and wraps with GORM's postgres dialector.
func Open(driver, dsn string, maxOpenConns, maxIdleConns int) (*gorm.DB, error) {
	mu.Lock()
	defer mu.Unlock()

	if gormDB != nil {
		return gormDB, nil
	}

	switch driver {
	case "sqlite", "":
		return openSQLite(dsn)
	case "postgres":
		return openPostgres(dsn, maxOpenConns, maxIdleConns)
	default:
		return nil, fmt.Errorf("db: unsupported driver %q", driver)
	}
}

// openSQLite opens a SQLite database using modernc.org/sqlite and wraps it with GORM.
// This avoids the sql.Register conflict between glebarez/sqlite and modernc.org/sqlite.
func openSQLite(dsn string) (*gorm.DB, error) {
	// Open using modernc.org/sqlite directly
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open sqlite: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	// Apply PRAGMAs before GORM wraps the connection
	if err := applySQLitePragmasRaw(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	// Wrap with GORM using the existing *sql.DB
	dialector := sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dsn,
		Conn:       sqlDB,
	}

	gormConfig := &gorm.Config{
		Logger:                 newGormLogrusLogger(),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: gorm open sqlite: %w", err)
	}

	gormDB = db
	log.Infof("db: sqlite database initialised")
	return gormDB, nil
}

// openPostgres opens a PostgreSQL database using pgx connection pool and wraps it with GORM.
func openPostgres(dsn string, maxOpenConns, maxIdleConns int) (*gorm.DB, error) {
	if maxOpenConns <= 0 {
		maxOpenConns = 25
	}
	if maxIdleConns <= 0 {
		maxIdleConns = 5
	}
	if maxIdleConns > maxOpenConns {
		maxIdleConns = maxOpenConns
	}

	dialector := postgres.Open(dsn)

	gormConfig := &gorm.Config{
		Logger:                 newGormLogrusLogger(),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("db: gorm open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db: get postgres underlying db: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	gormDB = db
	log.Infof("db: postgres database initialised (max_open=%d, max_idle=%d)", maxOpenConns, maxIdleConns)
	return gormDB, nil
}

// GetGORM returns the current GORM instance, or nil if not initialised.
func GetGORM() *gorm.DB {
	mu.Lock()
	defer mu.Unlock()
	return gormDB
}

// SetGORM sets the global GORM instance. Used by the usage package to
// initialize GORM on top of an existing *sql.DB connection.
func SetGORM(db *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	gormDB = db
}

// Close closes the GORM database connection.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	gormDB = nil
	return sqlDB.Close()
}

// IsSQLite returns true if the current GORM instance is backed by SQLite.
func IsSQLite() bool {
	d := GetGORM()
	if d == nil {
		return false
	}
	return IsSQLiteDB(d)
}

// IsSQLiteDB returns true if the given GORM DB is backed by SQLite.
func IsSQLiteDB(d *gorm.DB) bool {
	return d.Dialector.Name() == "sqlite"
}

// applySQLitePragmasRaw sets SQLite-specific PRAGMA values on a raw *sql.DB.
func applySQLitePragmasRaw(db *sql.DB) error {
	pragmas := []struct {
		sql   string
		fatal bool
	}{
		{sql: "PRAGMA busy_timeout = 5000", fatal: true},
		{sql: "PRAGMA journal_mode = WAL", fatal: false},
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p.sql); err != nil {
			if p.fatal {
				return fmt.Errorf("db: set pragma %q: %w", p.sql, err)
			}
			log.Warnf("db: failed to set pragma %q: %v", p.sql, err)
		}
	}
	return nil
}

// gormLogrusWriter adapts logrus to GORM's logger interface.
type gormLogrusWriter struct{}

func (w *gormLogrusWriter) Printf(format string, args ...interface{}) {
	log.Debugf(format, args...)
}

// newGormLogrusLogger creates a GORM logger that writes to logrus.
func newGormLogrusLogger() logger.Interface {
	return logger.New(
		&gormLogrusWriter{},
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}
