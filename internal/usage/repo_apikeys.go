package usage

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- GORM APIKey helpers ---

// apiKeyRowToGORM converts an APIKeyRow to a GORM APIKey model.
func apiKeyRowToGORM(row APIKeyRow) APIKey {
	return APIKey{
		ID:                   row.ID,
		Key:                  row.Key,
		Name:                 row.Name,
		Disabled:             row.Disabled,
		DailyLimit:           row.DailyLimit,
		TotalQuota:           row.TotalQuota,
		SpendingLimit:        row.SpendingLimit,
		ConcurrencyLimit:     row.ConcurrencyLimit,
		RPMLimit:             row.RPMLimit,
		TPMLimit:             row.TPMLimit,
		AllowedModels:        mustJSONStringList(row.AllowedModels),
		AllowedChannels:      mustJSONStringList(row.AllowedChannels),
		AllowedChannelGroups: mustJSONStringList(row.AllowedChannelGroups),
		SystemPrompt:         row.SystemPrompt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

// gormToAPIKeyRow converts a GORM APIKey model to an APIKeyRow.
func gormToAPIKeyRow(m APIKey) APIKeyRow {
	return APIKeyRow{
		ID:                   m.ID,
		Key:                  m.Key,
		Name:                 m.Name,
		Disabled:             m.Disabled,
		DailyLimit:           m.DailyLimit,
		TotalQuota:           m.TotalQuota,
		SpendingLimit:        m.SpendingLimit,
		ConcurrencyLimit:     m.ConcurrencyLimit,
		RPMLimit:             m.RPMLimit,
		TPMLimit:             m.TPMLimit,
		AllowedModels:        decodeJSONStringList(m.AllowedModels),
		AllowedChannels:      decodeJSONStringList(m.AllowedChannels),
		AllowedChannelGroups: decodeJSONStringList(m.AllowedChannelGroups),
		SystemPrompt:         m.SystemPrompt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// --- Gorm APIKey functions ---

// GormListAPIKeys retrieves all API key entries using GORM.
func GormListAPIKeys() []APIKeyRow {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}
	var keys []APIKey
	if err := gormDB.Order("created_at ASC").Find(&keys).Error; err != nil {
		log.Errorf("usage: GORM list api_keys: %v", err)
		return nil
	}
	result := make([]APIKeyRow, 0, len(keys))
	for _, k := range keys {
		result = append(result, gormToAPIKeyRow(k))
	}
	return result
}

// GormGetAPIKey retrieves a single API key entry by key string using GORM.
func GormGetAPIKey(key string) *APIKeyRow {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}
	var m APIKey
	if err := gormDB.Where("key = ?", key).First(&m).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Errorf("usage: GORM get api_key %s: %v", key, err)
		}
		return nil
	}
	row := gormToAPIKeyRow(m)
	return &row
}

// GormGetAPIKeyByID retrieves a single API key entry by numeric ID using GORM.
func GormGetAPIKeyByID(id int64) *APIKeyRow {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}
	var m APIKey
	if err := gormDB.Where("id = ?", id).First(&m).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Errorf("usage: GORM get api_key by id %d: %v", id, err)
		}
		return nil
	}
	row := gormToAPIKeyRow(m)
	return &row
}

// GormUpsertAPIKey inserts or updates an API key entry using GORM.
func GormUpsertAPIKey(entry APIKeyRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	trimmed := strings.TrimSpace(entry.Key)
	if trimmed == "" {
		return fmt.Errorf("key is required")
	}
	entry.Key = trimmed
	entry.Name = strings.TrimSpace(entry.Name)

	now := time.Now().UTC().Format(time.RFC3339)
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	m := apiKeyRowToGORM(entry)
	m.UpdatedAt = now

	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "disabled", "daily_limit", "total_quota", "spending_limit",
			"concurrency_limit", "rpm_limit", "tpm_limit",
			"allowed_models", "allowed_channels", "allowed_channel_groups",
			"system_prompt", "updated_at",
		}),
	}).Create(&m)

	if result.Error != nil {
		return fmt.Errorf("usage: GORM upsert api_key: %w", result.Error)
	}
	return nil
}

// GormDeleteAPIKey removes an API key entry by key string using GORM.
func GormDeleteAPIKey(key string) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}
	if err := gormDB.Where("key = ?", key).Delete(&APIKey{}).Error; err != nil {
		return fmt.Errorf("usage: GORM delete api_key: %w", err)
	}
	return nil
}

// GormDeleteAPIKeyByID removes an API key entry by numeric ID using GORM.
func GormDeleteAPIKeyByID(id int64) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}
	if err := gormDB.Where("id = ?", id).Delete(&APIKey{}).Error; err != nil {
		return fmt.Errorf("usage: GORM delete api_key by id: %w", err)
	}
	return nil
}

// GormReplaceAllAPIKeys atomically replaces all API keys using GORM.
func GormReplaceAllAPIKeys(entries []APIKeyRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&APIKey{}).Error; err != nil {
			return fmt.Errorf("usage: GORM clear api_keys: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		for _, entry := range entries {
			trimmed := strings.TrimSpace(entry.Key)
			if trimmed == "" {
				continue
			}
			entry.Key = trimmed
			entry.Name = strings.TrimSpace(entry.Name)
			if entry.CreatedAt == "" {
				entry.CreatedAt = now
			}
			entry.UpdatedAt = now

			m := apiKeyRowToGORM(entry)
			m.CreatedAt = entry.CreatedAt
			m.UpdatedAt = now
			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("usage: GORM insert api_key %s: %w", trimmed, err)
			}
		}
		return nil
	})
}

// GormMigrateAPIKeysFromConfig moves API key entries from YAML config into the database using GORM.
func GormMigrateAPIKeysFromConfig(cfg *config.Config, configFilePath string) int {
	gormDB := getGormDB()
	if gormDB == nil || cfg == nil {
		return 0
	}

	// Check if database already has data — skip if so (idempotent)
	var count int64
	if err := gormDB.Model(&APIKey{}).Count(&count).Error; err != nil {
		log.Errorf("usage: GORM migration count api_keys: %v", err)
		return 0
	}
	if count > 0 {
		cfg.APIKeys = nil
		cfg.APIKeyEntries = nil
		if configFilePath != "" {
			cleanAPIKeysFromYAML(configFilePath)
		}
		return 0
	}

	// Collect entries to migrate
	seen := make(map[string]struct{})
	var rows []APIKeyRow

	for _, entry := range cfg.APIKeyEntries {
		trimmed := strings.TrimSpace(entry.Key)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		row := APIKeyRowFromConfig(entry)
		row.Key = trimmed
		row.Name = strings.TrimSpace(row.Name)
		if row.Name == "" {
			row.Name = defaultAPIKeyName(len(rows))
		}
		if row.CreatedAt == "" {
			row.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		row.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		rows = append(rows, row)
	}

	for _, k := range cfg.APIKeys {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		row := APIKeyRow{
			Key:       trimmed,
			Name:      defaultAPIKeyName(len(rows)),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return 0
	}

	// Insert all rows with INSERT OR IGNORE semantics
	imported := 0
	for _, row := range rows {
		m := apiKeyRowToGORM(row)
		result := gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoNothing: true,
		}).Create(&m)
		if result.Error != nil {
			log.Errorf("usage: GORM api_keys migration insert %s: %v", row.Key, result.Error)
			continue
		}
		imported++
	}

	log.Infof("usage: GORM migrated %d API keys from config to database", imported)

	cfg.APIKeys = nil
	cfg.APIKeyEntries = nil

	if configFilePath != "" {
		if backupConfigForMigration(configFilePath, apiKeysMigrationBackupSuffix) {
			cleanAPIKeysFromYAML(configFilePath)
		}
	}

	return imported
}

// GormBackfillAPIKeyNames fills in missing API key names using GORM.
func GormBackfillAPIKeyNames() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	var keys []APIKey
	if err := gormDB.Where("trim(coalesce(name, '')) = ''").
		Order("created_at ASC, key ASC").
		Find(&keys).Error; err != nil {
		log.Warnf("usage: GORM query unnamed api_keys: %v", err)
		return
	}

	if len(keys) == 0 {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for idx, key := range keys {
		name := defaultAPIKeyName(idx)
		gormDB.Model(&APIKey{}).
			Where("key = ? AND trim(coalesce(name, '')) = ''", key.Key).
			Updates(map[string]interface{}{"name": name, "updated_at": now})
	}

	log.Infof("usage: GORM backfilled names for %d api_keys", len(keys))
}

// --- GORM APIKeyPermissionProfile helpers ---

// permissionProfileRowToGORM converts an APIKeyPermissionProfileRow to a GORM APIKeyPermissionProfile model.
func permissionProfileRowToGORM(row APIKeyPermissionProfileRow) APIKeyPermissionProfile {
	return APIKeyPermissionProfile{
		ID:                   row.ID,
		Name:                 row.Name,
		DailyLimit:           row.DailyLimit,
		TotalQuota:           row.TotalQuota,
		ConcurrencyLimit:     row.ConcurrencyLimit,
		RPMLimit:             row.RPMLimit,
		TPMLimit:             row.TPMLimit,
		AllowedModels:        mustJSONStringList(row.AllowedModels),
		AllowedChannels:      mustJSONStringList(row.AllowedChannels),
		AllowedChannelGroups: mustJSONStringList(row.AllowedChannelGroups),
		SystemPrompt:         row.SystemPrompt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}

// gormToPermissionProfileRow converts a GORM APIKeyPermissionProfile model to an APIKeyPermissionProfileRow.
func gormToPermissionProfileRow(m APIKeyPermissionProfile) APIKeyPermissionProfileRow {
	return APIKeyPermissionProfileRow{
		ID:                   m.ID,
		Name:                 m.Name,
		DailyLimit:           m.DailyLimit,
		TotalQuota:           m.TotalQuota,
		ConcurrencyLimit:     m.ConcurrencyLimit,
		RPMLimit:             m.RPMLimit,
		TPMLimit:             m.TPMLimit,
		AllowedModels:        decodeJSONStringList(m.AllowedModels),
		AllowedChannels:      decodeJSONStringList(m.AllowedChannels),
		AllowedChannelGroups: decodeJSONStringList(m.AllowedChannelGroups),
		SystemPrompt:         m.SystemPrompt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// --- Gorm APIKeyPermissionProfile functions ---

// GormListAPIKeyPermissionProfiles retrieves all permission profiles using GORM.
func GormListAPIKeyPermissionProfiles() []APIKeyPermissionProfileRow {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	var profiles []APIKeyPermissionProfile
	if err := gormDB.Order("created_at ASC, id ASC").Find(&profiles).Error; err != nil {
		log.Errorf("usage: GORM list api_key_permission_profiles: %v", err)
		return nil
	}

	result := make([]APIKeyPermissionProfileRow, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, gormToPermissionProfileRow(p))
	}
	return result
}

// GormReplaceAllAPIKeyPermissionProfiles atomically replaces all permission profiles using GORM.
func GormReplaceAllAPIKeyPermissionProfiles(profiles []APIKeyPermissionProfileRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&APIKeyPermissionProfile{}).Error; err != nil {
			return fmt.Errorf("usage: GORM clear api_key_permission_profiles: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		seen := make(map[string]struct{}, len(profiles))
		for _, profile := range profiles {
			profile = normalizeAPIKeyPermissionProfile(profile)
			if profile.ID == "" {
				return fmt.Errorf("id is required")
			}
			if profile.Name == "" {
				return fmt.Errorf("name is required")
			}
			if _, exists := seen[profile.ID]; exists {
				return fmt.Errorf("duplicate id %q", profile.ID)
			}
			seen[profile.ID] = struct{}{}
			if profile.CreatedAt == "" {
				profile.CreatedAt = now
			}
			profile.UpdatedAt = now

			m := permissionProfileRowToGORM(profile)
			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("usage: GORM insert permission profile %s: %w", profile.ID, err)
			}
		}
		return nil
	})
}

// GormMigrateAPIKeyPermissionProfilesFromYAML moves permission profiles from YAML config into the database using GORM.
func GormMigrateAPIKeyPermissionProfilesFromYAML(configFilePath string) int {
	gormDB := getGormDB()
	if gormDB == nil || strings.TrimSpace(configFilePath) == "" {
		return 0
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Warnf("usage: read config for api_key_permission_profiles migration: %v", err)
		return 0
	}

	var root struct {
		Profiles []APIKeyPermissionProfileRow `yaml:"api-key-permission-profiles"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		log.Warnf("usage: parse config for api_key_permission_profiles migration: %v", err)
		return 0
	}

	var count int64
	if err := gormDB.Model(&APIKeyPermissionProfile{}).Count(&count).Error; err != nil {
		log.Errorf("usage: GORM migration count api_key_permission_profiles: %v", err)
		return 0
	}
	if count > 0 {
		cleanAPIKeyPermissionProfilesFromYAML(configFilePath)
		return 0
	}

	profiles := make([]APIKeyPermissionProfileRow, 0, len(root.Profiles))
	for _, profile := range root.Profiles {
		profile = normalizeAPIKeyPermissionProfile(profile)
		if profile.ID == "" || profile.Name == "" {
			continue
		}
		profiles = append(profiles, profile)
	}
	if len(profiles) == 0 {
		cleanAPIKeyPermissionProfilesFromYAML(configFilePath)
		return 0
	}

	if err := GormReplaceAllAPIKeyPermissionProfiles(profiles); err != nil {
		log.Errorf("usage: GORM migrate api_key_permission_profiles: %v", err)
		return 0
	}

	if backupConfigForMigration(configFilePath, apiKeyPermissionProfilesMigrationBackupSuffix) {
		cleanAPIKeyPermissionProfilesFromYAML(configFilePath)
	}
	log.Infof("usage: GORM migrated %d API key permission profile(s) from config to database", len(profiles))
	return len(profiles)
}
