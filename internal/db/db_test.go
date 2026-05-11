package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestOpen_SQLite(t *testing.T) {
	// Reset global state
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	got, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got == nil {
		t.Fatal("Open() returned nil")
	}

	// Verify it's usable
	sqlDB, err := got.DB()
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	// Verify IsSQLite
	if !IsSQLiteDB(got) {
		t.Error("IsSQLiteDB() = false, want true")
	}
	if !IsSQLite() {
		t.Error("IsSQLite() = false, want true")
	}

	// Verify GetGORM returns the same instance
	if GetGORM() != got {
		t.Error("GetGORM() != got")
	}

	// Close and verify
	if err := Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if GetGORM() != nil {
		t.Error("GetGORM() != nil after Close()")
	}

	// Reset for next test
	gormDB = nil
}

func TestOpen_InvalidDriver(t *testing.T) {
	// Reset global state
	gormDB = nil

	_, err := Open("postgres", "host=localhost", 0, 0)
	if err == nil {
		t.Fatal("Open() with unsupported driver should return error")
	}

	gormDB = nil
}

func TestOpen_Idempotent(t *testing.T) {
	// Reset global state
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db1, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("First Open() error = %v", err)
	}

	db2, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Second Open() error = %v", err)
	}

	if db1 != db2 {
		t.Error("Open() should return the same instance on repeated calls")
	}

	gormDB = nil
}

func TestAutoMigrate(t *testing.T) {
	// Reset global state
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	d, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// Define a simple test model
	type TestModel struct {
		ID        int64  `gorm:"primaryKey;autoIncrement"`
		Name      string `gorm:"not null;default:''"`
		CreatedAt time.Time
	}

	err = AutoMigrate(d, &TestModel{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	// Verify the table exists
	if !d.Migrator().HasTable("test_models") {
		t.Error("Table test_models should exist after AutoMigrate")
	}

	// Test nil db
	err = AutoMigrate(nil, &TestModel{})
	if err == nil {
		t.Fatal("AutoMigrate(nil, ...) should return error")
	}

	gormDB = nil
}

func TestDateTruncExpr(t *testing.T) {
	// Reset global state
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	d, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// Just verify it returns a non-nil expression for known granularities
	for _, granularity := range []string{"day", "hour"} {
		expr := DateTruncExpr(d, "timestamp", granularity)
		if expr.SQL == "" {
			t.Errorf("DateTruncExpr(granularity=%q) returned empty SQL", granularity)
		}
	}

	gormDB = nil
}

func TestClose_WhenNotOpen(t *testing.T) {
	// Reset global state
	gormDB = nil

	err := Close()
	if err != nil {
		t.Fatalf("Close() when not open should not error, got %v", err)
	}
}

// TestSQLiteFileCreation verifies the database file is actually created on disk.
func TestSQLiteFileCreation(t *testing.T) {
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "creation_test.db")

	_, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// After open, the file should exist (SQLite creates it on first connection)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should exist after Open()")
	}

	gormDB = nil
}

// TestBasicCRUD verifies the GORM instance is usable for basic CRUD.
func TestBasicCRUD(t *testing.T) {
	gormDB = nil

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "crud_test.db")

	d, err := Open("sqlite", dbPath, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	type Sample struct {
		ID   int64  `gorm:"primaryKey;autoIncrement"`
		Name string `gorm:"not null"`
	}

	if err := AutoMigrate(d, &Sample{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	// Create
	s := Sample{Name: "test"}
	if err := d.Create(&s).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if s.ID == 0 {
		t.Error("Create() should auto-fill ID")
	}

	// Read
	var got Sample
	if err := d.First(&got, s.ID).Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.Name != "test" {
		t.Errorf("got.Name = %q, want %q", got.Name, "test")
	}

	// Update
	if err := d.Model(&got).Update("Name", "updated").Error; err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Delete
	if err := d.Delete(&got).Error; err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	err = d.First(&got, s.ID).Error
	if err == nil {
		t.Error("First() after Delete() should return error")
	}
	if err != gorm.ErrRecordNotFound {
		t.Errorf("expected ErrRecordNotFound, got %v", err)
	}

	gormDB = nil
}
