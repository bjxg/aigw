package usage

import (
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

type ModelConfigRow struct {
	ModelID               string  `json:"model_id"`
	OwnedBy               string  `json:"owned_by"`
	Description           string  `json:"description"`
	Enabled               bool    `json:"enabled"`
	PricingMode           string  `json:"pricing_mode"`
	InputPricePerMillion  float64 `json:"input_price_per_million"`
	OutputPricePerMillion float64 `json:"output_price_per_million"`
	CachedPricePerMillion float64 `json:"cached_price_per_million"`
	PricePerCall          float64 `json:"price_per_call"`
	Source                string  `json:"source"`
	UpdatedAt             string  `json:"updated_at"`
}

type ModelOwnerPresetRow struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	UpdatedAt   string `json:"updated_at"`
}

var (
	modelConfigCache   map[string]ModelConfigRow
	modelConfigCacheMu sync.RWMutex

	modelOwnerPresetCache   map[string]ModelOwnerPresetRow
	modelOwnerPresetCacheMu sync.RWMutex
)

var defaultOwnerLabels = map[string]string{
	"anthropic":    "Anthropic",
	"openai":       "OpenAI",
	"google":       "Google",
	"gemini":       "Gemini",
	"vertex":       "Vertex AI",
	"deepseek":     "DeepSeek",
	"qwen":         "Qwen",
	"kimi":         "Kimi",
	"minimax":      "MiniMax",
	"grok":         "Grok",
	"glm":          "GLM",
	"codex":        "Codex",
	"iflow":        "iFlow",
	"kiro":         "Kiro",
	"openrouter":   "OpenRouter",
	"azure-openai": "Azure OpenAI",
}

func normalizeModelOwnerValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "-"))
}

func normalizePricingMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "call") {
		return "call"
	}
	return "token"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intToBool(value int) bool {
	return value != 0
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func defaultModelConfigRows() []ModelConfigRow {
	channels := []string{
		"claude",
		"gemini",
		"vertex",
		"gemini-cli",
		"aistudio",
		"codex",
		"qwen",
		"iflow",
		"kimi",
		"antigravity",
	}

	seen := make(map[string]struct{})
	rows := make([]ModelConfigRow, 0, 256)
	for _, channel := range channels {
		for _, model := range registry.GetStaticModelDefinitionsByChannel(channel) {
			if model == nil || strings.TrimSpace(model.ID) == "" {
				continue
			}
			modelID := strings.TrimSpace(model.ID)
			if _, ok := seen[modelID]; ok {
				continue
			}
			seen[modelID] = struct{}{}

			ownedBy := normalizeModelOwnerValue(model.OwnedBy)
			if ownedBy == "" {
				ownedBy = normalizeModelOwnerValue(model.Type)
			}
			if ownedBy == "" {
				ownedBy = normalizeModelOwnerValue(channel)
			}
			description := strings.TrimSpace(model.Description)
			if description == "" {
				description = strings.TrimSpace(model.DisplayName)
			}

			row := ModelConfigRow{
				ModelID:     modelID,
				OwnedBy:     ownedBy,
				Description: description,
				Enabled:     true,
				PricingMode: "token",
				Source:      "seed",
			}
			if modelID == "gpt-image-2" {
				row.Description = "Image generation model billed per invocation"
				row.PricingMode = "call"
				row.PricePerCall = 0.04
			}
			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].ModelID) < strings.ToLower(rows[j].ModelID)
	})
	return rows
}

// gormSeedDefaultModelConfigRows seeds default model config and owner preset rows using GORM.
func gormSeedDefaultModelConfigRows() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	now := nowRFC3339()
	for _, row := range defaultModelConfigRows() {
		var existing ModelConfigRow
		result := gormDB.Table("model_configs").Where("model_id = ?", row.ModelID).First(&existing)
		if result.Error == nil {
			// Already exists, skip
			continue
		}
		gormDB.Table("model_configs").Create(map[string]interface{}{
			"model_id":                 row.ModelID,
			"owned_by":                 row.OwnedBy,
			"description":              row.Description,
			"enabled":                  boolToInt(row.Enabled),
			"pricing_mode":             normalizePricingMode(row.PricingMode),
			"input_price_per_million":  row.InputPricePerMillion,
			"output_price_per_million": row.OutputPricePerMillion,
			"cached_price_per_million": row.CachedPricePerMillion,
			"price_per_call":           row.PricePerCall,
			"source":                   row.Source,
			"updated_at":               now,
		})
	}

	for value, label := range defaultOwnerLabels {
		var existing ModelOwnerPresetRow
		result := gormDB.Table("model_owner_presets").Where("value = ?", value).First(&existing)
		if result.Error == nil {
			continue
		}
		gormDB.Table("model_owner_presets").Create(map[string]interface{}{
			"value":       value,
			"label":       label,
			"description": "",
			"enabled":     1,
			"updated_at":  now,
		})
	}
}

// mergeLegacyPricingIntoModelConfigs merges legacy model_pricing rows into model_configs.
func mergeLegacyPricingIntoModelConfigs() {
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	// Check if model_pricing table exists
	if !gormDB.Migrator().HasTable("model_pricing") {
		return
	}

	type legacyPricingRow struct {
		ModelID               string  `gorm:"column:model_id"`
		InputPricePerMillion  float64 `gorm:"column:input_price_per_million"`
		OutputPricePerMillion float64 `gorm:"column:output_price_per_million"`
		CachedPricePerMillion float64 `gorm:"column:cached_price_per_million"`
	}

	var legacyRows []legacyPricingRow
	result := gormDB.Raw(
		"SELECT model_id, input_price_per_million, output_price_per_million, cached_price_per_million FROM model_pricing",
	).Scan(&legacyRows)
	if result.Error != nil {
		log.Warnf("usage: merge legacy pricing: scan error: %v", result.Error)
		return
	}

	now := nowRFC3339()
	for _, row := range legacyRows {
		modelID := strings.TrimSpace(row.ModelID)
		if modelID == "" {
			continue
		}
		// Use raw SQL upsert for maximum compatibility
		if err := gormDB.Exec(
			`INSERT INTO model_configs (model_id, pricing_mode, input_price_per_million, output_price_per_million, cached_price_per_million, updated_at, owned_by, description, enabled, source)
			 VALUES (?, 'token', ?, ?, ?, ?, '', '', 1, 'legacy-pricing')
			 ON CONFLICT(model_id) DO UPDATE SET
			   pricing_mode = 'token',
			   input_price_per_million = excluded.input_price_per_million,
			   output_price_per_million = excluded.output_price_per_million,
			   cached_price_per_million = excluded.cached_price_per_million,
			   updated_at = excluded.updated_at`,
			modelID, row.InputPricePerMillion, row.OutputPricePerMillion, row.CachedPricePerMillion, now,
		).Error; err != nil {
			log.Warnf("usage: merge legacy pricing for %s: %v", modelID, err)
		}
	}
}

func ListModelConfigs() []ModelConfigRow {
	modelConfigCacheMu.RLock()
	defer modelConfigCacheMu.RUnlock()
	result := make([]ModelConfigRow, 0, len(modelConfigCache))
	for _, row := range modelConfigCache {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].ModelID) < strings.ToLower(result[j].ModelID)
	})
	return result
}

func GetModelConfig(modelID string) (ModelConfigRow, bool) {
	modelConfigCacheMu.RLock()
	defer modelConfigCacheMu.RUnlock()
	row, ok := modelConfigCache[strings.TrimSpace(modelID)]
	return row, ok
}

func UpsertModelConfig(row ModelConfigRow) error {
	return GormUpsertModelConfig(row)
}

func DeleteModelConfig(modelID string) error {
	return GormDeleteModelConfig(modelID)
}

func ownerLabelForValue(value string) string {
	value = normalizeModelOwnerValue(value)
	if label := defaultOwnerLabels[value]; label != "" {
		return label
	}
	parts := strings.Split(value, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func ListModelOwnerPresets() []ModelOwnerPresetRow {
	modelOwnerPresetCacheMu.RLock()
	defer modelOwnerPresetCacheMu.RUnlock()
	result := make([]ModelOwnerPresetRow, 0, len(modelOwnerPresetCache))
	for _, row := range modelOwnerPresetCache {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Value) < strings.ToLower(result[j].Value)
	})
	return result
}

func GetModelOwnerPreset(value string) (ModelOwnerPresetRow, bool) {
	modelOwnerPresetCacheMu.RLock()
	defer modelOwnerPresetCacheMu.RUnlock()
	row, ok := modelOwnerPresetCache[normalizeModelOwnerValue(value)]
	return row, ok
}

func UpsertModelOwnerPreset(row ModelOwnerPresetRow) error {
	return GormUpsertModelOwnerPreset(row)
}

func ReplaceModelOwnerPresets(rows []ModelOwnerPresetRow) error {
	return GormReplaceModelOwnerPresets(rows)
}
