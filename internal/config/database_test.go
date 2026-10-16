package config

import "testing"

func TestSanitizeDatabaseDoesNotFallbackToSQLite(t *testing.T) {
	for _, tt := range []struct{ driver, url, want string }{
		{"", "", "sqlite"},
		{" PostgreSQL ", "", "postgres"},
		{"postgres", "", "postgres"},
		{"sqlite", "postgresql://localhost/db", "postgres"},
		{"", "postgres://localhost/db", "postgres"},
		{"invalid", "", "invalid"},
	} {
		cfg := &Config{Database: DatabaseConfig{Driver: tt.driver, URL: tt.url}}
		cfg.SanitizeDatabase()
		if cfg.Database.Driver != tt.want {
			t.Errorf("driver %q, URL %q: got %q, want %q", tt.driver, tt.url, cfg.Database.Driver, tt.want)
		}
	}
}
