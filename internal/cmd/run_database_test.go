package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
)

func TestPostgresFailureStopsStartup(t *testing.T) {
	for _, driver := range []string{"postgres", "postgresql"} {
		for _, url := range []string{"", "postgres://localhost:invalid/database", "host=" + t.TempDir() + " dbname=test connect_timeout=1"} {
			t.Run(driver+"/"+url, func(t *testing.T) {
				cfg := &config.Config{Database: config.DatabaseConfig{Driver: driver, URL: url}}
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := StartService(cfg, path, ""); err == nil {
					t.Fatal("expected startup error")
				}
				cancel, done := StartServiceBackground(cfg, path, "")
				defer cancel()
				select {
				case <-done:
				default:
					t.Fatal("background startup did not stop")
				}
				if db.GetGORM() != nil {
					t.Fatal("unexpected database connection")
				}
				if _, err := os.Stat(filepath.Join(filepath.Dir(path), "data", "usage.db")); !os.IsNotExist(err) {
					t.Fatalf("unexpected SQLite file: %v", err)
				}
			})
		}
	}
}
