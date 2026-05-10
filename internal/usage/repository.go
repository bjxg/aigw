package usage

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

// LogRepository defines the interface for request log persistence operations.
type LogRepository interface {
	// Insert writes a request log and its content in a single transaction.
	Insert(ctx context.Context, log *RequestLog, inputContent, outputContent, detailContent string) error

	// Query returns a paginated, filtered list of log entries.
	Query(ctx context.Context, params LogQueryParams) (LogQueryResult, error)

	// DeleteByAPIKeyID removes all logs for the given API key ID.
	DeleteByAPIKeyID(ctx context.Context, apiKeyID int64) (int64, error)

	// QueryContent retrieves the stored request/response content for a single log entry.
	QueryContent(ctx context.Context, id int64) (LogContentResult, error)

	// QueryContentPart retrieves only one side (input/output/details) of the content.
	QueryContentPart(ctx context.Context, id int64, part string) (LogContentPartResult, error)

	// QueryContentForKey retrieves content only if the log belongs to the given API key ID.
	QueryContentForKey(ctx context.Context, id int64, apiKeyID int64) (LogContentResult, error)

	// QueryContentPartForKey retrieves one side of content, key-scoped by ID.
	QueryContentPartForKey(ctx context.Context, id int64, apiKeyID int64, part string) (LogContentPartResult, error)

	// QueryStats returns aggregated statistics over the filtered dataset.
	QueryStats(ctx context.Context, params LogQueryParams) (LogStats, error)

	// QueryFilters returns the distinct API keys and models within the time range.
	QueryFilters(ctx context.Context, days int) (FilterOptions, error)

	// QueryDashboardKPI returns aggregated KPI data for the dashboard.
	QueryDashboardKPI(ctx context.Context, days int) (DashboardKPI, error)

	// QueryDashboardTrends returns fixed-width trend buckets for the dashboard.
	QueryDashboardTrends(ctx context.Context, days int) (DashboardTrends, error)

	// QueryDailySeries returns per-day aggregated data for an API key.
	QueryDailySeries(ctx context.Context, apiKeyID int64, days int) ([]DailySeriesPoint, error)

	// QueryModelDistribution returns usage grouped by model for an API key.
	QueryModelDistribution(ctx context.Context, apiKeyID int64, days int) ([]ModelDistributionPoint, error)

	// QueryAPIKeyDistribution returns usage grouped by API key.
	QueryAPIKeyDistribution(ctx context.Context, days int) ([]APIKeyDistributionPoint, error)

	// QueryHourlySeries returns per-hour token and model aggregates.
	QueryHourlySeries(ctx context.Context, apiKeyID int64, hours int) ([]HourlyTokenPoint, []HourlyModelPoint, error)

	// QueryEntityStats returns aggregates grouped by a given column.
	QueryEntityStats(ctx context.Context, apiKeyID int64, days int, groupColumn string) ([]EntityStatPoint, error)

	// QueryLogStorageBytes returns approximate bytes occupied by stored content.
	QueryLogStorageBytes(ctx context.Context) (int64, error)

	// GetChannelAvgLatency returns average latency grouped by source.
	GetChannelAvgLatency(ctx context.Context, days int) ([]ChannelLatency, error)

	// CountTodayByKey returns the number of requests by a given API key ID today.
	CountTodayByKey(ctx context.Context, apiKeyID int64) (int64, error)

	// CountTotalByKey returns the total number of requests by a given API key ID.
	CountTotalByKey(ctx context.Context, apiKeyID int64) (int64, error)

	// QueryTotalCostByKey returns the total accumulated cost for a given API key ID.
	QueryTotalCostByKey(ctx context.Context, apiKeyID int64) (float64, error)

	// QueryModelsForKey returns distinct models used by an API key ID.
	QueryModelsForKey(ctx context.Context, apiKeyID int64, days int) ([]string, error)
}

// APIKeyRepository defines the interface for API key persistence operations.
type APIKeyRepository interface {
	List(ctx context.Context) ([]APIKeyRow, error)
	Get(ctx context.Context, key string) (*APIKeyRow, error)
	GetByID(ctx context.Context, id int64) (*APIKeyRow, error)
	Upsert(ctx context.Context, entry APIKeyRow) error
	Delete(ctx context.Context, key string) error
	DeleteByID(ctx context.Context, id int64) error
	ReplaceAll(ctx context.Context, entries []APIKeyRow) error
}

// ModelConfigRepository defines the interface for model config persistence.
type ModelConfigRepository interface {
	List(ctx context.Context) ([]ModelConfigRow, error)
	Get(ctx context.Context, modelID string) (ModelConfigRow, bool, error)
	Upsert(ctx context.Context, row ModelConfigRow) error
	Delete(ctx context.Context, modelID string) error
}

// ModelOwnerPresetRepository defines the interface for model owner preset persistence.
type ModelOwnerPresetRepository interface {
	List(ctx context.Context) ([]ModelOwnerPresetRow, error)
	Get(ctx context.Context, value string) (ModelOwnerPresetRow, bool, error)
	Upsert(ctx context.Context, row ModelOwnerPresetRow) error
	ReplaceAll(ctx context.Context, rows []ModelOwnerPresetRow) error
}

// ModelPricingRepository defines the interface for model pricing persistence.
type ModelPricingRepository interface {
	Get(ctx context.Context, modelID string) (ModelPricingRow, bool, error)
	GetAll(ctx context.Context) (map[string]ModelPricingRow, error)
	Upsert(ctx context.Context, modelID string, input, output, cached float64) error
	Delete(ctx context.Context, modelID string) error
}

// RuntimeSettingRepository defines the interface for runtime settings persistence.
type RuntimeSettingRepository interface {
	GetPayload(ctx context.Context, key string) ([]byte, bool, error)
	Exists(ctx context.Context, key string) (bool, error)
	Upsert(ctx context.Context, key string, value interface{}) error
}

// RoutingConfigRepository defines the interface for routing config persistence.
type RoutingConfigRepository interface {
	Get(ctx context.Context) ([]byte, error)
	Upsert(ctx context.Context, payload []byte) error
}

// ProxyPoolRepository defines the interface for proxy pool persistence.
type ProxyPoolRepository interface {
	List(ctx context.Context) ([]config.ProxyPoolEntry, error)
	Get(ctx context.Context, id string) (*config.ProxyPoolEntry, error)
	ReplaceAll(ctx context.Context, entries []config.ProxyPoolEntry) error
}

// PermissionProfileRepository defines the interface for API key permission profiles.
type PermissionProfileRepository interface {
	List(ctx context.Context) ([]APIKeyPermissionProfileRow, error)
	ReplaceAll(ctx context.Context, profiles []APIKeyPermissionProfileRow) error
}

// QuotaSnapshotRepository defines the interface for quota snapshot persistence.
type QuotaSnapshotRepository interface {
	RecordDaily(ctx context.Context, authIndex, provider string, quotas map[string]*float64) error
	RecordPoints(ctx context.Context, authIndex, provider string, points []QuotaSnapshotPoint) error
	QueryDailyByAuthIndexes(ctx context.Context, authIndexes []string, quotaKey string, days int) ([]DailyQuotaPoint, error)
	QueryPoints(ctx context.Context, authIndex string, start, end interface{}) ([]QuotaSnapshotPoint, error)
	QuerySeries(ctx context.Context, authIndex string, start, end interface{}) ([]QuotaSnapshotSeries, error)
}
