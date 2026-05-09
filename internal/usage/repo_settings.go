package usage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- GORM RuntimeSetting functions ---

// GormRuntimeSettingPayload retrieves the payload for a runtime setting key using GORM.
func GormRuntimeSettingPayload(key string) (json.RawMessage, bool) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, false
	}
	var setting RuntimeSetting
	if err := gormDB.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Warnf("usage: GORM load runtime setting %s: %v", key, err)
		}
		return nil, false
	}
	payload := strings.TrimSpace(setting.Payload)
	if payload == "" {
		payload = "{}"
	}
	return json.RawMessage(payload), true
}

// GormRuntimeSettingExists checks if a runtime setting key exists using GORM.
func GormRuntimeSettingExists(key string) bool {
	_, ok := GormRuntimeSettingPayload(key)
	return ok
}

// GormUpsertRuntimeSetting inserts or updates a runtime setting using GORM.
func GormUpsertRuntimeSetting(key string, value any) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	m := RuntimeSetting{
		SettingKey: key,
		Payload:    string(payload),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "setting_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"payload", "updated_at",
		}),
	}).Create(&m)

	return result.Error
}

// --- GORM RoutingConfig functions ---

// GormGetRoutingConfig retrieves the routing config using GORM.
func GormGetRoutingConfig() *config.RoutingConfig {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	var rc RoutingConfig
	if err := gormDB.Where("id = 1").First(&rc).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Warnf("usage: GORM load routing_config: %v", err)
		}
		return nil
	}

	payload := strings.TrimSpace(rc.Payload)
	if payload == "" {
		return nil
	}

	var cfg config.RoutingConfig
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		log.Warnf("usage: GORM decode routing_config: %v", err)
		return nil
	}
	normalized := normalizeRoutingConfig(cfg)
	return &normalized
}

// GormUpsertRoutingConfig inserts or updates the routing config using GORM.
func GormUpsertRoutingConfig(cfg config.RoutingConfig) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	normalized := normalizeRoutingConfig(cfg)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	m := RoutingConfig{
		ID:        1,
		Payload:   string(payload),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"payload", "updated_at",
		}),
	}).Create(&m)

	return result.Error
}

// --- GORM ProxyPool functions ---

// GormListProxyPool retrieves all proxy pool entries using GORM.
func GormListProxyPool() []config.ProxyPoolEntry {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	var entries []ProxyPool
	if err := gormDB.Order("created_at ASC, id ASC").Find(&entries).Error; err != nil {
		log.Errorf("usage: GORM list proxy_pool: %v", err)
		return nil
	}

	result := make([]config.ProxyPoolEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, config.ProxyPoolEntry{
			ID:          e.ID,
			Name:        e.Name,
			URL:         e.URL,
			Enabled:     e.Enabled,
			Description: e.Description,
		})
	}
	return result
}

// GormGetProxyPoolEntry retrieves one proxy pool entry by ID using GORM.
func GormGetProxyPoolEntry(id string) *config.ProxyPoolEntry {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	normalizedID := normalizeProxyPoolEntryID(id)
	if normalizedID == "" {
		return nil
	}

	var entry ProxyPool
	if err := gormDB.Where("id = ?", normalizedID).First(&entry).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Warnf("usage: GORM get proxy_pool %s: %v", normalizedID, err)
		}
		return nil
	}

	return &config.ProxyPoolEntry{
		ID:          entry.ID,
		Name:        entry.Name,
		URL:         entry.URL,
		Enabled:     entry.Enabled,
		Description: entry.Description,
	}
}

// GormReplaceProxyPool atomically replaces all proxy pool entries using GORM.
func GormReplaceProxyPool(entries []config.ProxyPoolEntry) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	normalized := config.NormalizeProxyPool(entries)

	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&ProxyPool{}).Error; err != nil {
			return fmt.Errorf("usage: GORM clear proxy_pool: %w", err)
		}

		if len(normalized) == 0 {
			return nil
		}

		now := time.Now().UTC().Format(time.RFC3339)
		for _, entry := range normalized {
			m := ProxyPool{
				ID:          entry.ID,
				Name:        entry.Name,
				URL:         entry.URL,
				Enabled:     entry.Enabled,
				Description: entry.Description,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("usage: GORM insert proxy_pool %s: %w", entry.ID, err)
			}
		}
		return nil
	})
}
