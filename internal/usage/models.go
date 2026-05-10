package usage

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// --- RequestLog ---

// RequestLog maps to the request_logs table.
type RequestLog struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Timestamp       time.Time `gorm:"index:idx_logs_timestamp;not null" json:"timestamp"`
	APIKeyID        int64     `gorm:"index:idx_logs_api_key_id;not null;default:0" json:"api_key_id"`
	APIKeyName      string    `gorm:"not null;default:''" json:"api_key_name"`
	Model           string    `gorm:"index:idx_logs_model;not null;default:''" json:"model"`
	Source          string    `gorm:"not null;default:''" json:"source"`
	ChannelName     string    `gorm:"not null;default:''" json:"channel_name"`
	AuthIndex       string    `gorm:"index:idx_logs_auth_index;not null;default:''" json:"auth_index"`
	Failed          bool      `gorm:"index:idx_logs_failed;not null;default:false" json:"failed"`
	LatencyMs       int64     `gorm:"not null;default:0" json:"latency_ms"`
	FirstTokenMs    int64     `gorm:"not null;default:0" json:"first_token_ms"`
	InputTokens     int64     `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens    int64     `gorm:"not null;default:0" json:"output_tokens"`
	ReasoningTokens int64     `gorm:"not null;default:0" json:"reasoning_tokens"`
	CachedTokens    int64     `gorm:"not null;default:0" json:"cached_tokens"`
	TotalTokens     int64     `gorm:"not null;default:0" json:"total_tokens"`
	Cost            float64   `gorm:"not null;default:0" json:"cost"`
	InputContent    string    `gorm:"not null;default:''" json:"-"` // legacy inline content, kept for migration
	OutputContent   string    `gorm:"not null;default:''" json:"-"` // legacy inline content, kept for migration
}

// TableName overrides the table name.
func (RequestLog) TableName() string { return "request_logs" }

// --- RequestLogContent ---

// RequestLogContent maps to the request_log_content table.
type RequestLogContent struct {
	LogID         int64  `gorm:"primaryKey" json:"log_id"`
	Timestamp     time.Time `gorm:"index:idx_log_content_timestamp;not null" json:"timestamp"`
	Compression   string `gorm:"not null;default:'zstd'" json:"compression"`
	InputContent  []byte `gorm:"type:blob;not null;default:''" json:"input_content"`
	OutputContent []byte `gorm:"type:blob;not null;default:''" json:"output_content"`
	DetailContent []byte `gorm:"type:blob;not null;default:''" json:"detail_content"`
}

// TableName overrides the table name.
func (RequestLogContent) TableName() string { return "request_log_content" }

// --- APIKey ---

// APIKey maps to the api_keys table.
type APIKey struct {
	ID                   int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	Key                  string  `gorm:"uniqueIndex;not null" json:"key"`
	Name                 string  `gorm:"not null;default:''" json:"name"`
	Disabled             bool    `gorm:"not null;default:false" json:"disabled"`
	DailyLimit           int     `gorm:"not null;default:0" json:"daily_limit"`
	TotalQuota           int     `gorm:"not null;default:0" json:"total_quota"`
	SpendingLimit        float64 `gorm:"not null;default:0" json:"spending_limit"`
	ConcurrencyLimit     int     `gorm:"not null;default:0" json:"concurrency_limit"`
	RPMLimit             int     `gorm:"not null;default:0" json:"rpm_limit"`
	TPMLimit             int     `gorm:"not null;default:0" json:"tpm_limit"`
	AllowedModels        string  `gorm:"not null;default:'[]'" json:"allowed_models"`         // JSON-encoded []string
	AllowedChannels      string  `gorm:"not null;default:'[]'" json:"allowed_channels"`       // JSON-encoded []string
	AllowedChannelGroups string  `gorm:"not null;default:'[]'" json:"allowed_channel_groups"` // JSON-encoded []string
	SystemPrompt         string  `gorm:"not null;default:''" json:"system_prompt"`
	CreatedAt            string  `gorm:"not null;default:''" json:"created_at"`
	UpdatedAt            string  `gorm:"not null;default:''" json:"updated_at"`
}

// TableName overrides the table name.
func (APIKey) TableName() string { return "api_keys" }

// --- APIKeyPermissionProfile ---

// APIKeyPermissionProfile maps to the api_key_permission_profiles table.
type APIKeyPermissionProfile struct {
	ID                   string `gorm:"primaryKey;not null" json:"id"`
	Name                 string `gorm:"not null;default:''" json:"name"`
	DailyLimit           int    `gorm:"not null;default:0" json:"daily_limit"`
	TotalQuota           int    `gorm:"not null;default:0" json:"total_quota"`
	ConcurrencyLimit     int    `gorm:"not null;default:0" json:"concurrency_limit"`
	RPMLimit             int    `gorm:"not null;default:0" json:"rpm_limit"`
	TPMLimit             int    `gorm:"not null;default:0" json:"tpm_limit"`
	AllowedModels        string `gorm:"not null;default:'[]'" json:"allowed_models"`
	AllowedChannels      string `gorm:"not null;default:'[]'" json:"allowed_channels"`
	AllowedChannelGroups string `gorm:"not null;default:'[]'" json:"allowed_channel_groups"`
	SystemPrompt         string `gorm:"not null;default:''" json:"system_prompt"`
	CreatedAt            string `gorm:"not null;default:''" json:"created_at"`
	UpdatedAt            string `gorm:"not null;default:''" json:"updated_at"`
}

// TableName overrides the table name.
func (APIKeyPermissionProfile) TableName() string { return "api_key_permission_profiles" }

// --- ModelConfig ---

// ModelConfig maps to the model_configs table.
type ModelConfig struct {
	ModelID               string  `gorm:"primaryKey;not null" json:"model_id"`
	OwnedBy               string  `gorm:"index:idx_model_configs_owned_by;not null;default:''" json:"owned_by"`
	Description            string  `gorm:"not null;default:''" json:"description"`
	Enabled               bool    `gorm:"not null;default:true" json:"enabled"`
	PricingMode           string  `gorm:"not null;default:'token'" json:"pricing_mode"`
	InputPricePerMillion  float64 `gorm:"not null;default:0" json:"input_price_per_million"`
	OutputPricePerMillion float64 `gorm:"not null;default:0" json:"output_price_per_million"`
	CachedPricePerMillion float64 `gorm:"not null;default:0" json:"cached_price_per_million"`
	PricePerCall          float64 `gorm:"not null;default:0" json:"price_per_call"`
	Source                string  `gorm:"not null;default:'user'" json:"source"`
	UpdatedAt             string  `gorm:"not null" json:"updated_at"`
}

// TableName overrides the table name.
func (ModelConfig) TableName() string { return "model_configs" }

// --- ModelOwnerPreset ---

// ModelOwnerPreset maps to the model_owner_presets table.
type ModelOwnerPreset struct {
	Value       string `gorm:"primaryKey;not null" json:"value"`
	Label       string `gorm:"not null;default:''" json:"label"`
	Description string `gorm:"not null;default:''" json:"description"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	UpdatedAt   string `gorm:"not null" json:"updated_at"`
}

// TableName overrides the table name.
func (ModelOwnerPreset) TableName() string { return "model_owner_presets" }

// --- ModelPricing ---

// ModelPricing maps to the model_pricing table.
type ModelPricing struct {
	ModelID               string  `gorm:"primaryKey;not null" json:"model_id"`
	InputPricePerMillion  float64 `gorm:"not null;default:0" json:"input_price_per_million"`
	OutputPricePerMillion float64 `gorm:"not null;default:0" json:"output_price_per_million"`
	CachedPricePerMillion float64 `gorm:"not null;default:0" json:"cached_price_per_million"`
	UpdatedAt             string  `gorm:"not null" json:"updated_at"`
}

// TableName overrides the table name.
func (ModelPricing) TableName() string { return "model_pricing" }

// --- ModelOpenRouterSyncState ---

// ModelOpenRouterSyncState maps to the model_openrouter_sync_state table.
// This is a singleton table (id is always 1).
type ModelOpenRouterSyncState struct {
	ID              int    `gorm:"primaryKey;not null;check:id = 1" json:"id"`
	Enabled         bool   `gorm:"not null;default:false" json:"enabled"`
	IntervalMinutes int    `gorm:"not null;default:1440" json:"interval_minutes"`
	LastSyncAt      string `gorm:"not null;default:''" json:"last_sync_at"`
	LastSuccessAt   string `gorm:"not null;default:''" json:"last_success_at"`
	LastError       string `gorm:"not null;default:''" json:"last_error"`
	LastSeen        int    `gorm:"not null;default:0" json:"last_seen"`
	LastAdded       int    `gorm:"not null;default:0" json:"last_added"`
	LastUpdated     int    `gorm:"not null;default:0" json:"last_updated"`
	LastSkipped     int    `gorm:"not null;default:0" json:"last_skipped"`
	UpdatedAt       string `gorm:"not null" json:"updated_at"`
}

// TableName overrides the table name.
func (ModelOpenRouterSyncState) TableName() string { return "model_openrouter_sync_state" }

// --- RoutingConfig ---

// RoutingConfig maps to the routing_config table.
// This is a singleton table (id is always 1).
type RoutingConfig struct {
	ID        int    `gorm:"primaryKey;not null;check:id = 1" json:"id"`
	Payload   string `gorm:"not null;default:'{}'" json:"payload"`
	UpdatedAt string `gorm:"not null;default:''" json:"updated_at"`
}

// TableName overrides the table name.
func (RoutingConfig) TableName() string { return "routing_config" }

// --- ProxyPool ---

// ProxyPool maps to the proxy_pool table.
type ProxyPool struct {
	ID          string `gorm:"primaryKey;not null" json:"id"`
	Name        string `gorm:"not null;default:''" json:"name"`
	URL         string `gorm:"not null" json:"url"`
	Enabled     bool   `gorm:"not null;default:true" json:"enabled"`
	Description string `gorm:"not null;default:''" json:"description"`
	CreatedAt   string `gorm:"not null;default:''" json:"created_at"`
	UpdatedAt   string `gorm:"not null;default:''" json:"updated_at"`
}

// TableName overrides the table name.
func (ProxyPool) TableName() string { return "proxy_pool" }

// --- RuntimeSetting ---

// RuntimeSetting maps to the runtime_settings table.
type RuntimeSetting struct {
	SettingKey string `gorm:"primaryKey;not null" json:"setting_key"`
	Payload    string `gorm:"not null;default:'{}'" json:"payload"`
	UpdatedAt  string `gorm:"not null;default:''" json:"updated_at"`
}

// TableName overrides the table name.
func (RuntimeSetting) TableName() string { return "runtime_settings" }

// --- AuthFileQuotaSnapshot ---

// AuthFileQuotaSnapshot maps to the auth_file_quota_snapshots table.
type AuthFileQuotaSnapshot struct {
	DateKey    string   `gorm:"primaryKey;not null;index:idx_quota_snapshots_date" json:"date_key"`
	AuthIndex  string   `gorm:"primaryKey;not null;index:idx_quota_snapshots_auth" json:"auth_index"`
	QuotaKey   string   `gorm:"primaryKey;not null" json:"quota_key"`
	Provider   string   `gorm:"not null;default:''" json:"provider"`
	Percent    *float64 `gorm:"" json:"percent"`
	RecordedAt string   `gorm:"not null" json:"recorded_at"`
}

// TableName overrides the table name.
func (AuthFileQuotaSnapshot) TableName() string { return "auth_file_quota_snapshots" }

// --- AuthFileQuotaSnapshotPoint ---

// AuthFileQuotaSnapshotPoint maps to the auth_file_quota_snapshot_points table.
type AuthFileQuotaSnapshotPoint struct {
	ID            int64    `gorm:"primaryKey;autoIncrement" json:"id"`
	RecordedAt    string   `gorm:"not null;index:idx_quota_snapshot_points_auth_time;index:idx_quota_snapshot_points_auth_key_time" json:"recorded_at"`
	AuthIndex     string   `gorm:"not null;default:'';index:idx_quota_snapshot_points_auth_time;index:idx_quota_snapshot_points_auth_key_time" json:"auth_index"`
	Provider      string   `gorm:"not null;default:''" json:"provider"`
	QuotaKey      string   `gorm:"not null;index:idx_quota_snapshot_points_auth_key_time" json:"quota_key"`
	QuotaLabel    string   `gorm:"not null;default:''" json:"quota_label"`
	Percent       *float64 `gorm:"" json:"percent"`
	ResetAt       *string  `gorm:"" json:"reset_at"`
	WindowSeconds int64    `gorm:"not null;default:0" json:"window_seconds"`
}

// TableName overrides the table name.
func (AuthFileQuotaSnapshotPoint) TableName() string { return "auth_file_quota_snapshot_points" }

// --- JSONStringList helper type ---

// JSONStringList is a helper type for storing []string as JSON in TEXT columns.
// It implements driver.Valuer and sql.Scanner.
type JSONStringList []string

// Value implements driver.Valuer.
func (l JSONStringList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	data, err := json.Marshal([]string(l))
	if err != nil {
		return "[]", err
	}
	return string(data), nil
}

// Scan implements sql.Scanner.
func (l *JSONStringList) Scan(value interface{}) error {
	if value == nil {
		*l = []string{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("usage: cannot scan %T into JSONStringList", value)
	}
	var result []string
	if err := json.Unmarshal(bytes, &result); err != nil {
		*l = []string{}
		return nil
	}
	*l = result
	return nil
}

// AllModels returns all GORM model instances for auto-migration.
func AllModels() []interface{} {
	return []interface{}{
		&RequestLog{},
		&RequestLogContent{},
		&APIKey{},
		&APIKeyPermissionProfile{},
		&ModelConfig{},
		&ModelOwnerPreset{},
		&ModelPricing{},
		&ModelOpenRouterSyncState{},
		&RoutingConfig{},
		&ProxyPool{},
		&RuntimeSetting{},
		&AuthFileQuotaSnapshot{},
		&AuthFileQuotaSnapshotPoint{},
	}
}

// Ensure JSONStringList implements the right interfaces.
var _ driver.Valuer = JSONStringList(nil)
var _ interface{ Scan(interface{}) error } = (*JSONStringList)(nil)


