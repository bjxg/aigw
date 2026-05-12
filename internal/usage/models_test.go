package usage

import (
	"database/sql"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	_ "modernc.org/sqlite"
)

// setupGormTestDB creates an in-memory SQLite GORM instance for testing.
// It uses modernc.org/sqlite directly to avoid driver registration conflicts.
func setupGormTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	dialector := sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        ":memory:",
		Conn:       sqlDB,
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open GORM: %v", err)
	}
	return gormDB
}

func TestAllModels_ReturnsAllModels(t *testing.T) {
	models := AllModels()
	if len(models) != 12 {
		t.Errorf("AllModels() returned %d models, want 12", len(models))
	}
}

func TestAutoMigrate_AllModels(t *testing.T) {
	d := setupGormTestDB(t)

	err := db.AutoMigrate(d, AllModels()...)
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	// Verify each table exists
	expectedTables := []string{
		"request_logs",
		"request_log_content",
		"api_keys",
		"api_key_permission_profiles",
		"model_configs",
		"model_owner_presets",
		"model_pricing",
		"model_openrouter_sync_state",
		"routing_config",
		"proxy_pool",
		"runtime_settings",
	}

	for _, table := range expectedTables {
		if !d.Migrator().HasTable(table) {
			t.Errorf("table %q should exist after AutoMigrate", table)
		}
	}
}

func TestRequestLog_CRUD(t *testing.T) {
	d := setupGormTestDB(t)
	err := db.AutoMigrate(d, &RequestLog{}, &RequestLogContent{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	// Create
	logEntry := &RequestLog{
		APIKeyID:    1,
		APIKeyName:  "Test Key",
		Model:       "gpt-4",
		Source:      "openai",
		ChannelName: "claude",
		AuthIndex:   "0",
		Timestamp:   parseTime(t, "2025-01-01T00:00:00Z"),
	}
	if err := d.Create(logEntry).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if logEntry.ID == 0 {
		t.Error("Create() should auto-fill ID (primary key backfill)")
	}

	// Read
	var got RequestLog
	if err := d.First(&got, logEntry.ID).Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.APIKeyID != 1 {
		t.Errorf("got.APIKeyID = %d, want %d", got.APIKeyID, 1)
	}
	if got.Model != "gpt-4" {
		t.Errorf("got.Model = %q, want %q", got.Model, "gpt-4")
	}

	// Create associated content
	content := &RequestLogContent{
		LogID:         logEntry.ID,
		Timestamp:     logEntry.Timestamp,
		Compression:   "zstd",
		InputContent:  []byte("compressed-input"),
		OutputContent: []byte("compressed-output"),
	}
	if err := d.Create(content).Error; err != nil {
		t.Fatalf("Create content() error = %v", err)
	}

	// Read content
	var gotContent RequestLogContent
	if err := d.First(&gotContent, logEntry.ID).Error; err != nil {
		t.Fatalf("First content() error = %v", err)
	}
	if string(gotContent.InputContent) != "compressed-input" {
		t.Errorf("gotContent.InputContent = %q, want %q", string(gotContent.InputContent), "compressed-input")
	}
}

func TestAPIKey_CRUD(t *testing.T) {
	d := setupGormTestDB(t)
	err := db.AutoMigrate(d, &APIKey{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	key := &APIKey{
		Key:           "sk-test123",
		Name:          "Test Key",
		AllowedModels: `["gpt-4"]`,
		CreatedAt:     "2025-01-01T00:00:00Z",
		UpdatedAt:     "2025-01-01T00:00:00Z",
	}
	if err := d.Create(key).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var got APIKey
	if err := d.First(&got, "key = ?", "sk-test123").Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.Name != "Test Key" {
		t.Errorf("got.Name = %q, want %q", got.Name, "Test Key")
	}
}

func TestModelConfig_CRUD(t *testing.T) {
	d := setupGormTestDB(t)
	err := db.AutoMigrate(d, &ModelConfig{}, &ModelPricing{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	config := &ModelConfig{
		ModelID:               "gpt-4",
		OwnedBy:               "openai",
		Enabled:               true,
		PricingMode:           "token",
		InputPricePerMillion:  30.0,
		OutputPricePerMillion: 60.0,
		Source:                "user",
		UpdatedAt:             "2025-01-01T00:00:00Z",
	}
	if err := d.Create(config).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var got ModelConfig
	if err := d.First(&got, "model_id = ?", "gpt-4").Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.InputPricePerMillion != 30.0 {
		t.Errorf("got.InputPricePerMillion = %v, want 30.0", got.InputPricePerMillion)
	}
}

func TestRuntimeSetting_CRUD(t *testing.T) {
	d := setupGormTestDB(t)
	err := db.AutoMigrate(d, &RuntimeSetting{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	setting := &RuntimeSetting{
		SettingKey: "test-key",
		Payload:    `{"value": "test"}`,
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	if err := d.Create(setting).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var got RuntimeSetting
	if err := d.First(&got, "setting_key = ?", "test-key").Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.Payload != `{"value": "test"}` {
		t.Errorf("got.Payload = %q, want %q", got.Payload, `{"value": "test"}`)
	}
}

func TestRoutingConfig_CRUD(t *testing.T) {
	d := setupGormTestDB(t)
	err := db.AutoMigrate(d, &RoutingConfig{})
	if err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	rc := &RoutingConfig{
		ID:        1,
		Payload:   `{"strategy": "round-robin"}`,
		UpdatedAt: "2025-01-01T00:00:00Z",
	}
	if err := d.Create(rc).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var got RoutingConfig
	if err := d.First(&got, 1).Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.Payload != `{"strategy": "round-robin"}` {
		t.Errorf("got.Payload = %q, want round-robin payload", got.Payload)
	}
}

func TestJSONStringList(t *testing.T) {
	tests := []struct {
		name  string
		input JSONStringList
		want  string
	}{
		{"nil", nil, "[]"},
		{"empty", JSONStringList{}, "[]"},
		{"single", JSONStringList{"a"}, `["a"]`},
		{"multiple", JSONStringList{"a", "b"}, `["a","b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Value() = %q, want %q", got, tt.want)
			}

			// Round-trip: scan the value back
			var result JSONStringList
			if err := result.Scan(got); err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if len(result) != len(tt.input) {
				t.Errorf("Scan() result length = %d, want %d", len(result), len(tt.input))
			}
		})
	}
}

func TestJSONStringList_ScanNil(t *testing.T) {
	var l JSONStringList
	if err := l.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if len(l) != 0 {
		t.Errorf("Scan(nil) result = %v, want empty slice", l)
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parseTime(%q) error = %v", s, err)
	}
	return parsed
}
