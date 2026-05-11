package usage

import (
	"sync"
	"time"
)

// ModelPricingRow represents a single model's pricing configuration.
type ModelPricingRow struct {
	ModelID               string  `json:"model_id"`
	InputPricePerMillion  float64 `json:"input_price_per_million"`
	OutputPricePerMillion float64 `json:"output_price_per_million"`
	CachedPricePerMillion float64 `json:"cached_price_per_million"`
	UpdatedAt             string  `json:"updated_at"`
}

// In-memory pricing cache for fast cost calculation.
var (
	pricingCache   map[string]ModelPricingRow
	pricingCacheMu sync.RWMutex
)

// UpsertModelPricing inserts or updates a model's pricing and refreshes the cache.
func UpsertModelPricing(modelID string, input, output, cached float64) error {
	if err := GormUpsertModelPricing(modelID, input, output, cached); err != nil {
		return err
	}

	// Update in-memory cache
	now := time.Now().UTC().Format(time.RFC3339)
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

	// Sync into model_configs table
	gormUpsertLegacyPricingIntoModelConfig(modelID, input, output, cached, now)
	return nil
}

// GetModelPricing returns the pricing for a single model.
func GetModelPricing(modelID string) (ModelPricingRow, bool) {
	pricingCacheMu.RLock()
	defer pricingCacheMu.RUnlock()
	row, ok := pricingCache[modelID]
	return row, ok
}

// GetAllModelPricing returns all model pricing entries.
func GetAllModelPricing() map[string]ModelPricingRow {
	pricingCacheMu.RLock()
	defer pricingCacheMu.RUnlock()
	result := make(map[string]ModelPricingRow, len(pricingCache))
	for k, v := range pricingCache {
		result[k] = v
	}
	return result
}

// DeleteModelPricing removes a model's pricing.
func DeleteModelPricing(modelID string) error {
	if err := GormDeleteModelPricing(modelID); err != nil {
		return err
	}
	pricingCacheMu.Lock()
	delete(pricingCache, modelID)
	pricingCacheMu.Unlock()
	return nil
}

func calculateTokenCost(inputTokens, outputTokens, cachedTokens int64, inputPrice, outputPrice, cachedPrice float64) float64 {
	billableInputTokens := inputTokens
	if cachedTokens > 0 && inputTokens >= cachedTokens {
		billableInputTokens = inputTokens - cachedTokens
	}
	if cachedPrice <= 0 {
		cachedPrice = inputPrice
	}
	return float64(billableInputTokens)/1_000_000*inputPrice +
		float64(outputTokens)/1_000_000*outputPrice +
		float64(cachedTokens)/1_000_000*cachedPrice
}
