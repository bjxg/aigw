package usage

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- GORM ModelConfig helpers ---

func modelConfigRowToGORM(row ModelConfigRow) ModelConfig {
	return ModelConfig{
		ModelID:               row.ModelID,
		OwnedBy:               row.OwnedBy,
		Description:            row.Description,
		Enabled:               row.Enabled,
		PricingMode:           row.PricingMode,
		InputPricePerMillion:  row.InputPricePerMillion,
		OutputPricePerMillion: row.OutputPricePerMillion,
		CachedPricePerMillion: row.CachedPricePerMillion,
		PricePerCall:          row.PricePerCall,
		Source:                row.Source,
		UpdatedAt:             row.UpdatedAt,
	}
}

func gormToModelConfigRow(m ModelConfig) ModelConfigRow {
	return ModelConfigRow{
		ModelID:               m.ModelID,
		OwnedBy:               m.OwnedBy,
		Description:           m.Description,
		Enabled:               m.Enabled,
		PricingMode:           m.PricingMode,
		InputPricePerMillion:  m.InputPricePerMillion,
		OutputPricePerMillion: m.OutputPricePerMillion,
		CachedPricePerMillion: m.CachedPricePerMillion,
		PricePerCall:          m.PricePerCall,
		Source:                m.Source,
		UpdatedAt:             m.UpdatedAt,
	}
}

// --- GORM ModelOwnerPreset helpers ---

func modelOwnerPresetRowToGORM(row ModelOwnerPresetRow) ModelOwnerPreset {
	return ModelOwnerPreset{
		Value:       row.Value,
		Label:       row.Label,
		Description: row.Description,
		Enabled:     row.Enabled,
		UpdatedAt:   row.UpdatedAt,
	}
}

func gormToModelOwnerPresetRow(m ModelOwnerPreset) ModelOwnerPresetRow {
	return ModelOwnerPresetRow{
		Value:       m.Value,
		Label:       m.Label,
		Description: m.Description,
		Enabled:     m.Enabled,
		UpdatedAt:   m.UpdatedAt,
	}
}

// --- GORM ModelPricing helpers ---

func modelPricingRowToGORM(row ModelPricingRow) ModelPricing {
	return ModelPricing{
		ModelID:               row.ModelID,
		InputPricePerMillion:  row.InputPricePerMillion,
		OutputPricePerMillion: row.OutputPricePerMillion,
		CachedPricePerMillion: row.CachedPricePerMillion,
		UpdatedAt:             row.UpdatedAt,
	}
}

func gormToModelPricingRow(m ModelPricing) ModelPricingRow {
	return ModelPricingRow{
		ModelID:               m.ModelID,
		InputPricePerMillion:  m.InputPricePerMillion,
		OutputPricePerMillion: m.OutputPricePerMillion,
		CachedPricePerMillion: m.CachedPricePerMillion,
		UpdatedAt:             m.UpdatedAt,
	}
}

// --- Gorm ModelConfig functions ---

// GormUpsertModelConfig inserts or updates a model config using GORM.
func GormUpsertModelConfig(row ModelConfigRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}
	row.ModelID = strings.TrimSpace(row.ModelID)
	if row.ModelID == "" {
		return fmt.Errorf("usage: model id is required")
	}
	row.OwnedBy = normalizeModelOwnerValue(row.OwnedBy)
	row.PricingMode = normalizePricingMode(row.PricingMode)
	if row.Source == "" {
		row.Source = "user"
	}
	row.UpdatedAt = nowRFC3339()

	m := modelConfigRowToGORM(row)

	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"owned_by", "description", "enabled", "pricing_mode",
			"input_price_per_million", "output_price_per_million",
			"cached_price_per_million", "price_per_call",
			"source", "updated_at",
		}),
	}).Create(&m)

	if result.Error != nil {
		return fmt.Errorf("usage: GORM upsert model config: %w", result.Error)
	}

	if row.PricingMode == "token" {
		if err := GormUpsertModelPricing(row.ModelID, row.InputPricePerMillion, row.OutputPricePerMillion, row.CachedPricePerMillion); err != nil {
			return err
		}
	} else if err := GormDeleteModelPricing(row.ModelID); err != nil {
		return err
	}
	if row.OwnedBy != "" {
		if err := GormUpsertModelOwnerPreset(ModelOwnerPresetRow{
			Value:   row.OwnedBy,
			Label:   ownerLabelForValue(row.OwnedBy),
			Enabled: true,
		}); err != nil {
			return err
		}
	}
	gormReloadModelConfigCache()
	return nil
}

// GormDeleteModelConfig removes a model config using GORM.
func GormDeleteModelConfig(modelID string) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("usage: model id is required")
	}
	if err := gormDB.Where("model_id = ?", modelID).Delete(&ModelConfig{}).Error; err != nil {
		return fmt.Errorf("usage: GORM delete model config: %w", err)
	}
	if err := GormDeleteModelPricing(modelID); err != nil {
		return err
	}
	gormReloadModelConfigCache()
	return nil
}

// GormSeedDefaultModelConfigRows seeds default model config rows using GORM.
func GormSeedDefaultModelConfigRows() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	now := nowRFC3339()
	for _, row := range defaultModelConfigRows() {
		m := modelConfigRowToGORM(row)
		m.UpdatedAt = now
		result := gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "model_id"}},
			DoNothing: true,
		}).Create(&m)
		if result.Error != nil {
			log.Warnf("usage: GORM seed model config %s: %v", row.ModelID, result.Error)
		}
	}

	for value, label := range defaultOwnerLabels {
		m := ModelOwnerPreset{
			Value:     value,
			Label:     label,
			Enabled:   true,
			UpdatedAt: now,
		}
		result := gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "value"}},
			DoNothing: true,
		}).Create(&m)
		if result.Error != nil {
			log.Warnf("usage: GORM seed owner preset %s: %v", value, result.Error)
		}
	}

	// Seed owner presets from existing model configs
	var owners []string
	gormDB.Model(&ModelConfig{}).Where("owned_by != ''").Distinct("owned_by").Pluck("owned_by", &owners)
	for _, owner := range owners {
		value := normalizeModelOwnerValue(owner)
		if value == "" {
			continue
		}
		label := defaultOwnerLabels[value]
		if label == "" {
			label = owner
		}
		m := ModelOwnerPreset{
			Value:     value,
			Label:     label,
			Enabled:   true,
			UpdatedAt: now,
		}
		gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "value"}},
			DoNothing: true,
		}).Create(&m)
	}
}

// GormMergeLegacyPricingIntoModelConfigs merges pricing data into model_configs using GORM.
func GormMergeLegacyPricingIntoModelConfigs() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	var pricings []ModelPricing
	if err := gormDB.Find(&pricings).Error; err != nil {
		return
	}

	now := nowRFC3339()
	for _, p := range pricings {
		modelID := strings.TrimSpace(p.ModelID)
		if modelID == "" {
			continue
		}
		m := ModelConfig{
			ModelID:               modelID,
			OwnedBy:               "",
			Description:           "",
			Enabled:               true,
			PricingMode:           "token",
			InputPricePerMillion:  p.InputPricePerMillion,
			OutputPricePerMillion: p.OutputPricePerMillion,
			CachedPricePerMillion: p.CachedPricePerMillion,
			PricePerCall:          0,
			Source:                "legacy-pricing",
			UpdatedAt:             now,
		}
		gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "model_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"pricing_mode", "input_price_per_million", "output_price_per_million",
				"cached_price_per_million", "updated_at",
			}),
		}).Create(&m)
	}
}

// --- Gorm cache reload functions ---

// gormReloadModelConfigCache reloads the model config cache from GORM.
func gormReloadModelConfigCache() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	var configs []ModelConfig
	if err := gormDB.Find(&configs).Error; err != nil {
		log.Errorf("usage: GORM load model config cache: %v", err)
		return
	}

	cache := make(map[string]ModelConfigRow, len(configs))
	for _, c := range configs {
		row := gormToModelConfigRow(c)
		row.PricingMode = normalizePricingMode(row.PricingMode)
		cache[row.ModelID] = row
	}

	modelConfigCacheMu.Lock()
	modelConfigCache = cache
	modelConfigCacheMu.Unlock()
	log.Infof("usage: GORM loaded %d model config entries into cache", len(cache))
}

// gormReloadModelOwnerPresetCache reloads the model owner preset cache from GORM.
func gormReloadModelOwnerPresetCache() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	var presets []ModelOwnerPreset
	if err := gormDB.Find(&presets).Error; err != nil {
		log.Errorf("usage: GORM load model owner preset cache: %v", err)
		return
	}

	cache := make(map[string]ModelOwnerPresetRow, len(presets))
	for _, p := range presets {
		row := gormToModelOwnerPresetRow(p)
		row.Value = normalizeModelOwnerValue(row.Value)
		cache[row.Value] = row
	}

	modelOwnerPresetCacheMu.Lock()
	modelOwnerPresetCache = cache
	modelOwnerPresetCacheMu.Unlock()
	log.Infof("usage: GORM loaded %d model owner presets into cache", len(cache))
}

// gormReloadPricingCache reloads the pricing cache from GORM.
func gormReloadPricingCache() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	var pricings []ModelPricing
	if err := gormDB.Find(&pricings).Error; err != nil {
		log.Errorf("usage: GORM load pricing cache: %v", err)
		return
	}

	cache := make(map[string]ModelPricingRow, len(pricings))
	for _, p := range pricings {
		cache[p.ModelID] = gormToModelPricingRow(p)
	}

	pricingCacheMu.Lock()
	pricingCache = cache
	pricingCacheMu.Unlock()
	log.Infof("usage: GORM loaded %d model pricing entries into cache", len(cache))
}

// --- Gorm ModelOwnerPreset functions ---

// GormUpsertModelOwnerPreset inserts or updates a model owner preset using GORM.
func GormUpsertModelOwnerPreset(row ModelOwnerPresetRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}
	row.Value = normalizeModelOwnerValue(row.Value)
	if row.Value == "" {
		return fmt.Errorf("usage: owner value is required")
	}
	if strings.TrimSpace(row.Label) == "" {
		row.Label = ownerLabelForValue(row.Value)
	}
	row.UpdatedAt = nowRFC3339()

	m := modelOwnerPresetRowToGORM(row)
	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "value"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"label", "description", "enabled", "updated_at",
		}),
	}).Create(&m)

	if result.Error != nil {
		return fmt.Errorf("usage: GORM upsert owner preset: %w", result.Error)
	}
	gormReloadModelOwnerPresetCache()
	return nil
}

// GormReplaceModelOwnerPresets atomically replaces all model owner presets using GORM.
func GormReplaceModelOwnerPresets(rows []ModelOwnerPresetRow) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}

	err := gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&ModelOwnerPreset{}).Error; err != nil {
			return fmt.Errorf("usage: GORM clear owner presets: %w", err)
		}

		now := nowRFC3339()
		for _, row := range rows {
			row.Value = normalizeModelOwnerValue(row.Value)
			if row.Value == "" {
				continue
			}
			if strings.TrimSpace(row.Label) == "" {
				row.Label = ownerLabelForValue(row.Value)
			}
			m := modelOwnerPresetRowToGORM(row)
			m.UpdatedAt = now
			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("usage: GORM insert owner preset: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	gormReloadModelOwnerPresetCache()
	return nil
}

// --- Gorm ModelPricing functions ---

// GormUpsertModelPricing inserts or updates model pricing using GORM.
func GormUpsertModelPricing(modelID string, input, output, cached float64) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m := ModelPricing{
		ModelID:               modelID,
		InputPricePerMillion:  input,
		OutputPricePerMillion: output,
		CachedPricePerMillion: cached,
		UpdatedAt:             now,
	}

	result := gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"input_price_per_million", "output_price_per_million",
			"cached_price_per_million", "updated_at",
		}),
	}).Create(&m)

	if result.Error != nil {
		return fmt.Errorf("usage: GORM upsert pricing: %w", result.Error)
	}

	// Update in-memory cache
	pricingCacheMu.Lock()
	if pricingCache == nil {
		pricingCache = make(map[string]ModelPricingRow)
	}
	pricingCache[modelID] = ModelPricingRow{
		ModelID:               modelID,
		InputPricePerMillion:  input,
		OutputPricePerMillion: output,
		CachedPricePerMillion: cached,
		UpdatedAt:             now,
	}
	pricingCacheMu.Unlock()

	// Sync into model_configs
	gormUpsertLegacyPricingIntoModelConfig(modelID, input, output, cached, now)
	return nil
}

// GormDeleteModelPricing removes a model's pricing using GORM.
func GormDeleteModelPricing(modelID string) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("usage: database not initialised")
	}
	if err := gormDB.Where("model_id = ?", modelID).Delete(&ModelPricing{}).Error; err != nil {
		return fmt.Errorf("usage: GORM delete pricing: %w", err)
	}
	pricingCacheMu.Lock()
	delete(pricingCache, modelID)
	pricingCacheMu.Unlock()
	return nil
}

// gormUpsertLegacyPricingIntoModelConfig syncs pricing data into model_configs using GORM.
func gormUpsertLegacyPricingIntoModelConfig(modelID string, input, output, cached float64, updatedAt string) {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return
	}

	m := ModelConfig{
		ModelID:               modelID,
		OwnedBy:               "",
		Description:           "",
		Enabled:               true,
		PricingMode:           "token",
		InputPricePerMillion:  input,
		OutputPricePerMillion: output,
		CachedPricePerMillion: cached,
		PricePerCall:          0,
		Source:                "legacy-pricing",
		UpdatedAt:             updatedAt,
	}

	gormDB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"pricing_mode", "input_price_per_million", "output_price_per_million",
			"cached_price_per_million", "price_per_call", "updated_at",
		}),
	}).Create(&m)

	gormReloadModelConfigCache()
}

// --- Gorm OpenRouterSyncState functions ---

// GormGetOpenRouterModelSyncState retrieves the OpenRouter sync state using GORM.
func GormGetOpenRouterModelSyncState() OpenRouterModelSyncState {
	gormDB := getGormDB()
	state := OpenRouterModelSyncState{
		IntervalMinutes: defaultOpenRouterModelSyncIntervalMinutes,
		Running:         openRouterSyncRunning.Load(),
	}
	if gormDB == nil {
		return state
	}

	GormEnsureOpenRouterModelSyncStateRow()

	var m ModelOpenRouterSyncState
	if err := gormDB.Where("id = 1").First(&m).Error; err != nil {
		return state
	}

	state.Enabled = m.Enabled
	state.IntervalMinutes = normalizeOpenRouterModelSyncInterval(m.IntervalMinutes)
	state.LastSyncAt = m.LastSyncAt
	state.LastSuccessAt = m.LastSuccessAt
	state.LastError = m.LastError
	state.LastSeen = m.LastSeen
	state.LastAdded = m.LastAdded
	state.LastUpdated = m.LastUpdated
	state.LastSkipped = m.LastSkipped
	state.UpdatedAt = m.UpdatedAt
	state.Running = openRouterSyncRunning.Load()
	return state
}

// GormUpdateOpenRouterModelSyncSettings updates the OpenRouter sync settings using GORM.
func GormUpdateOpenRouterModelSyncSettings(enabled bool, intervalMinutes int) (OpenRouterModelSyncState, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return OpenRouterModelSyncState{}, fmt.Errorf("usage: database not initialised")
	}

	GormEnsureOpenRouterModelSyncStateRow()

	now := nowRFC3339()
	if err := gormDB.Model(&ModelOpenRouterSyncState{}).
		Where("id = 1").
		Updates(map[string]interface{}{
			"enabled":          enabled,
			"interval_minutes": normalizeOpenRouterModelSyncInterval(intervalMinutes),
			"updated_at":       now,
		}).Error; err != nil {
		return OpenRouterModelSyncState{}, fmt.Errorf("usage: GORM update openrouter sync settings: %w", err)
	}

	return GormGetOpenRouterModelSyncState(), nil
}

// GormEnsureOpenRouterModelSyncStateRow ensures the singleton row exists using GORM.
func GormEnsureOpenRouterModelSyncStateRow() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	m := ModelOpenRouterSyncState{
		ID:              1,
		Enabled:         false,
		IntervalMinutes: defaultOpenRouterModelSyncIntervalMinutes,
		LastSyncAt:      "",
		LastSuccessAt:   "",
		LastError:       "",
		LastSeen:        0,
		LastAdded:       0,
		LastUpdated:     0,
		LastSkipped:     0,
		UpdatedAt:       nowRFC3339(),
	}
	gormDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&m)
}

// GormRecordOpenRouterModelSyncResult records the sync result using GORM.
func GormRecordOpenRouterModelSyncResult(result OpenRouterModelSyncResult, syncErr error) OpenRouterModelSyncState {
	gormDB := getGormDB()
	if gormDB == nil {
		return GormGetOpenRouterModelSyncState()
	}

	GormEnsureOpenRouterModelSyncStateRow()
	now := nowRFC3339()
	state := GormGetOpenRouterModelSyncState()

	lastSuccessAt := state.LastSuccessAt
	lastError := ""
	if syncErr != nil {
		lastError = syncErr.Error()
	} else {
		lastSuccessAt = now
	}

	gormDB.Model(&ModelOpenRouterSyncState{}).Where("id = 1").Updates(map[string]interface{}{
		"last_sync_at":    now,
		"last_success_at": lastSuccessAt,
		"last_error":      lastError,
		"last_seen":       result.Seen,
		"last_added":      result.Added,
		"last_updated":    result.Updated,
		"last_skipped":    result.Skipped,
		"updated_at":      now,
	})

	return GormGetOpenRouterModelSyncState()
}

// GormSeedDefaultModelConfigRowsV2 seeds default model configs and owner presets using GORM.
// This is the GORM equivalent of seedDefaultModelConfigRows.
func GormSeedDefaultModelConfigRowsV2() {
	GormSeedDefaultModelConfigRows()
}

// GormInitModelConfigTables initializes model config tables using GORM.
func GormInitModelConfigTables() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}
	GormSeedDefaultModelConfigRows()
	GormMergeLegacyPricingIntoModelConfigs()
	gormReloadModelConfigCache()
	gormReloadModelOwnerPresetCache()
}
