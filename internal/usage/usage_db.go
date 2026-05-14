package usage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogRow represents a single request log entry returned by QueryLogs.
type LogRow struct {
	ID              int64     `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	APIKeyID        int64     `json:"api_key_id"`
	APIKeyName      string    `json:"api_key_name"`
	UserID          *int64    `json:"user_id,omitempty"`
	Model           string    `json:"model"`
	Source          string    `json:"source"`
	ChannelName     string    `json:"channel_name"`
	AuthIndex       string    `json:"auth_index"`
	Failed          bool      `json:"failed"`
	LatencyMs       int64     `json:"latency_ms"`
	FirstTokenMs    int64     `json:"first_token_ms"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	Cost            float64   `json:"cost"`
	HasContent      bool      `json:"has_content"`
}

// LogQueryParams holds filter/pagination parameters for QueryLogs.
type LogQueryParams struct {
	Page         int      // 1-based
	Size         int      // rows per page
	Days         int      // time range in days
	APIKeyID     int64    // filter by api_key_id; 0 = no filter, -1 = system requests (api_key_id = 0)
	UserID       int64    // filter by user_id; 0 = no filter
	Model        string   // exact match filter
	Status       string   // "success", "failed", or "" (all)
	AuthIndexes  []string // optional auth_index IN (...) filter
	ChannelNames []string // optional channel_name IN (...) filter
}

// LogQueryResult holds the paginated query result.
type LogQueryResult struct {
	Items []LogRow `json:"items"`
	Total int64    `json:"total"`
	Page  int      `json:"page"`
	Size  int      `json:"size"`
}

// APIKeyFilterItem holds an API key ID and name for filter dropdowns.
type APIKeyFilterItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// UserFilterItem holds a user ID and name for filter dropdowns.
type UserFilterItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// FilterOptions holds the available filter values for the UI.
type FilterOptions struct {
	APIKeys  []APIKeyFilterItem `json:"api_keys"`
	Users    []UserFilterItem   `json:"users"`
	Models   []string           `json:"models"`
	Channels []string           `json:"channels"`
}

// LogStats holds aggregated stats over the filtered result set.
type LogStats struct {
	Total       int64   `json:"total"`
	SuccessRate float64 `json:"success_rate"`
	TotalTokens int64   `json:"total_tokens"`
	TotalCost   float64 `json:"total_cost"`
}

type DailyCountPoint struct {
	Date     string `json:"date"`
	Requests int64  `json:"requests"`
}

type HourlyCountPoint struct {
	Hour     string `json:"hour"`
	Requests int64  `json:"requests"`
}

const systemRequestLogFilterValue = "__system__"

var (
	usageDBMu   sync.Mutex
	usageDBPath string
	usageLoc    *time.Location
)

// getSQLDB returns the underlying *sql.DB from the GORM instance.
// This is useful for tests and for SQLite-specific maintenance operations.
func getSQLDB() *sql.DB {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil
	}
	return sqlDB
}

// InsertLog writes a single request log entry into the database.
// It is safe to call concurrently.
func InsertLog(apiKeyID int64, apiKeyName, model, source, channelName, authIndex string,
	failed bool, timestamp time.Time, latencyMs, firstTokenMs int64, tokens TokenStats,
	inputContent, outputContent string) {
	GormInsertLog(apiKeyID, apiKeyName, nil, model, source, channelName, authIndex, failed, timestamp, latencyMs, firstTokenMs, tokens, inputContent, outputContent, "")
}

func InsertLogWithDetails(apiKeyID int64, apiKeyName, model, source, channelName, authIndex string,
	failed bool, timestamp time.Time, latencyMs, firstTokenMs int64, tokens TokenStats,
	inputContent, outputContent, detailContent string) {
	GormInsertLog(apiKeyID, apiKeyName, nil, model, source, channelName, authIndex, failed, timestamp, latencyMs, firstTokenMs, tokens, inputContent, outputContent, detailContent)
}

// InsertLogWithUserID writes a request log with an associated user_id.
func InsertLogWithUserID(apiKeyID int64, apiKeyName string, userID *int64, model, source, channelName, authIndex string,
	failed bool, timestamp time.Time, latencyMs, firstTokenMs int64, tokens TokenStats,
	inputContent, outputContent, detailContent string) {
	GormInsertLog(apiKeyID, apiKeyName, userID, model, source, channelName, authIndex, failed, timestamp, latencyMs, firstTokenMs, tokens, inputContent, outputContent, detailContent)
}

// tokenUsageCallback is set by SetTokenUsageCallback to notify external
// rate limiters (e.g. quota middleware) of token consumption.
var tokenUsageCallback func(apiKey string, totalTokens int64)

// SetTokenUsageCallback registers a function to be called after each
// request's tokens are recorded. Used by the quota middleware for TPM tracking.
func SetTokenUsageCallback(fn func(apiKey string, totalTokens int64)) {
	tokenUsageCallback = fn
}

// QueryLogs returns a paginated, filtered list of log entries.
func QueryLogs(params LogQueryParams) (LogQueryResult, error) {
	return GormQueryLogs(params)
}

// QueryFilters returns the distinct API keys and models within the time range.
func QueryFilters(days int) (FilterOptions, error) {
	return GormQueryFilters(days)
}

// QueryStats returns aggregated statistics over the filtered dataset.
func QueryStats(params LogQueryParams) (LogStats, error) {
	return GormQueryStats(params)
}

// DeleteLogsByAPIKeyID removes all request_logs and request_log_content entries
// for the given API key ID. Returns the number of deleted log rows.
func DeleteLogsByAPIKeyID(apiKeyID int64) (int64, error) {
	return GormDeleteLogsByAPIKeyID(apiKeyID)
}

// DeleteLogsByAPIKey is retained for backward compatibility.
// It looks up the API key by string and delegates to DeleteLogsByAPIKeyID.
func DeleteLogsByAPIKey(apiKey string) (int64, error) {
	row := GetAPIKey(apiKey)
	if row == nil {
		return 0, fmt.Errorf("usage: api_key not found: %s", apiKey)
	}
	return DeleteLogsByAPIKeyID(row.ID)
}

// DashboardKPI holds the aggregated KPI data needed by the dashboard page.
type DashboardKPI struct {
	TotalRequests   int64   `json:"total_requests"`
	SuccessRequests int64   `json:"success_requests"`
	FailedRequests  int64   `json:"failed_requests"`
	SuccessRate     float64 `json:"success_rate"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
}

type DashboardTrendPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type DashboardThroughputPoint struct {
	Label string  `json:"label"`
	RPM   float64 `json:"rpm"`
	TPM   float64 `json:"tpm"`
}

type DashboardTrends struct {
	RequestVolume    []DashboardTrendPoint      `json:"request_volume"`
	SuccessRate      []DashboardTrendPoint      `json:"success_rate"`
	TotalTokens      []DashboardTrendPoint      `json:"total_tokens"`
	FailedRequests   []DashboardTrendPoint      `json:"failed_requests"`
	ThroughputSeries []DashboardThroughputPoint `json:"throughput_series"`
}

// QueryDashboardKPI returns aggregated KPI data for the dashboard.
func QueryDashboardKPI(days int) (DashboardKPI, error) {
	return GormQueryDashboardKPI(days)
}

// QueryDashboardTrends returns fixed-width trend buckets used by the dashboard.
func QueryDashboardTrends(days int) (DashboardTrends, error) {
	return GormQueryDashboardTrends(days)
}

func emptyDashboardTrends(days int) DashboardTrends {
	if days < 1 {
		days = 7
	}
	loc := getUsageLocation()
	trends := dashboardTrendsFromBuckets(buildDashboardBuckets(days, loc))
	trends.ThroughputSeries = throughputSeriesFromBuckets(buildRecentThroughputBucketsAt(time.Now(), loc))
	return trends
}

// MigrateFromSnapshot is retained for API compatibility but no longer
// migrates individual request details as they are no longer stored in memory.
func MigrateFromSnapshot(snapshot StatisticsSnapshot) (int64, error) {
	return 0, nil
}

// --- internal helpers ---

func getUsageLocation() *time.Location {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()
	if usageLoc == nil {
		return time.Local
	}
	return usageLoc
}

func cutoffStartUTCAt(now time.Time, days int) time.Time {
	if days < 1 {
		days = 7
	}
	loc := getUsageLocation()
	now = now.In(loc)
	todayStartLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return todayStartLocal.AddDate(0, 0, -(days - 1)).UTC()
}

// CutoffStartUTC returns the start-of-day cutoff for the given number of days
// in the project-configured timezone, converted to UTC. Exported so that
// dashboard and other callers can reuse the same time-range semantics.
func CutoffStartUTC(days int) time.Time {
	return cutoffStartUTCAt(time.Now(), days)
}

func localDayKeyAt(t time.Time) string {
	loc := getUsageLocation()
	return t.In(loc).Format("2006-01-02")
}

// LocalDayKeyAt returns the YYYY-MM-DD day key in the project-configured timezone.
func LocalDayKeyAt(t time.Time) string {
	return localDayKeyAt(t)
}

func cutoffDayKey(days int) string {
	return localDayKeyAt(CutoffStartUTC(days))
}

// --- Dashboard bucket helpers (shared between raw and GORM paths) ---

type dashboardBucket struct {
	label      string
	key        string
	minutes    float64
	requests   int64
	success    int64
	failed     int64
	totalToken int64
}

const dashboardThroughputBucketCount = 7

func buildDashboardBuckets(days int, loc *time.Location) []dashboardBucket {
	if loc == nil {
		loc = time.Local
	}
	start := CutoffStartUTC(days).In(loc)
	if days == 1 {
		buckets := make([]dashboardBucket, 0, 24)
		for i := 0; i < 24; i++ {
			at := start.Add(time.Duration(i) * time.Hour)
			buckets = append(buckets, dashboardBucket{
				label:   at.Format("15:04"),
				key:     dashboardBucketKey(at, days),
				minutes: 60,
			})
		}
		return buckets
	}

	buckets := make([]dashboardBucket, 0, days)
	for i := 0; i < days; i++ {
		at := start.AddDate(0, 0, i)
		buckets = append(buckets, dashboardBucket{
			label:   at.Format("2006-01-02"),
			key:     dashboardBucketKey(at, days),
			minutes: 24 * 60,
		})
	}
	return buckets
}

func dashboardBucketKey(t time.Time, days int) string {
	if days == 1 {
		return t.Format("2006-01-02 15")
	}
	return t.Format("2006-01-02")
}

func buildRecentThroughputBucketsAt(now time.Time, loc *time.Location) []dashboardBucket {
	if loc == nil {
		loc = time.Local
	}
	currentMinute := now.In(loc).Truncate(time.Minute)
	start := currentMinute.Add(-time.Duration(dashboardThroughputBucketCount-1) * time.Minute)
	buckets := make([]dashboardBucket, 0, dashboardThroughputBucketCount)
	for i := 0; i < dashboardThroughputBucketCount; i++ {
		at := start.Add(time.Duration(i) * time.Minute)
		buckets = append(buckets, dashboardBucket{
			label:   at.Format("15:04"),
			key:     at.Format("2006-01-02 15:04"),
			minutes: 1,
		})
	}
	return buckets
}

func dashboardTrendsFromBuckets(buckets []dashboardBucket) DashboardTrends {
	trends := DashboardTrends{
		RequestVolume:    make([]DashboardTrendPoint, 0, len(buckets)),
		SuccessRate:      make([]DashboardTrendPoint, 0, len(buckets)),
		TotalTokens:      make([]DashboardTrendPoint, 0, len(buckets)),
		FailedRequests:   make([]DashboardTrendPoint, 0, len(buckets)),
		ThroughputSeries: make([]DashboardThroughputPoint, 0),
	}

	for _, bucket := range buckets {
		successRate := 0.0
		if bucket.requests > 0 {
			successRate = float64(bucket.success) / float64(bucket.requests) * 100
		}

		trends.RequestVolume = append(trends.RequestVolume, DashboardTrendPoint{Label: bucket.label, Value: float64(bucket.requests)})
		trends.SuccessRate = append(trends.SuccessRate, DashboardTrendPoint{Label: bucket.label, Value: successRate})
		trends.TotalTokens = append(trends.TotalTokens, DashboardTrendPoint{Label: bucket.label, Value: float64(bucket.totalToken)})
		trends.FailedRequests = append(trends.FailedRequests, DashboardTrendPoint{Label: bucket.label, Value: float64(bucket.failed)})
	}

	return trends
}

func throughputSeriesFromBuckets(buckets []dashboardBucket) []DashboardThroughputPoint {
	points := make([]DashboardThroughputPoint, 0, len(buckets))
	for _, bucket := range buckets {
		rpm := 0.0
		tpm := 0.0
		if bucket.minutes > 0 {
			rpm = float64(bucket.requests) / bucket.minutes
			tpm = float64(bucket.totalToken) / bucket.minutes
		}
		points = append(points, DashboardThroughputPoint{
			Label: bucket.label,
			RPM:   rpm,
			TPM:   tpm,
		})
	}
	return points
}

// QueryDailySeriesForUser returns per-day aggregated request count and token usage for a given user.
func QueryDailySeriesForUser(userID int64, apiKeyID int64, days int) ([]DailySeriesPoint, error) {
	return GormQueryDailySeriesForUser(userID, apiKeyID, days)
}

// QueryModelDistributionForUser returns request count and token usage grouped by model for a given user.
func QueryModelDistributionForUser(userID int64, apiKeyID int64, days int) ([]ModelDistributionPoint, error) {
	return GormQueryModelDistributionForUser(userID, apiKeyID, days)
}

// QueryModelsForUser returns the distinct models used by a specific user within the time range.
func QueryModelsForUser(userID int64, days int) ([]string, error) {
	return GormQueryModelsForUser(userID, days)
}

// QueryLogContentForUser retrieves log content for a single entry, but only if it belongs to the given user.
func QueryLogContentForUser(id int64, userID int64) (LogContentResult, error) {
	return GormQueryLogContentForUser(id, userID)
}

// QueryLogContentPartForUser retrieves only one side (input/output) of the stored request/response content
// for a single entry, but only if it belongs to the given user.
func QueryLogContentPartForUser(id int64, userID int64, part string) (LogContentPartResult, error) {
	return GormQueryLogContentPartForUser(id, userID, part)
}

// LogContentResult holds the content detail for a single log entry.
type LogContentResult struct {
	ID            int64  `json:"id"`
	InputContent  string `json:"input_content"`
	OutputContent string `json:"output_content"`
	Model         string `json:"model"`
}

// LogContentPartResult holds one side (input/output) of the content detail for a single log entry.
type LogContentPartResult struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
	Model   string `json:"model"`
	Part    string `json:"part"`
}

func normalizeLogContentPart(part string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "input":
		return "input", nil
	case "output":
		return "output", nil
	case "details":
		return "details", nil
	default:
		return "", fmt.Errorf("usage: invalid content part %q", part)
	}
}

// QueryLogContent retrieves the stored request/response content for a single log entry.
func QueryLogContent(id int64) (LogContentResult, error) {
	return GormQueryLogContent(id)
}

// QueryLogContentPart retrieves only one side (input/output) of the stored request/response content
// for a single log entry.
func QueryLogContentPart(id int64, part string) (LogContentPartResult, error) {
	return GormQueryLogContentPart(id, part)
}

// DailySeriesPoint holds one day of aggregated usage data.
type DailySeriesPoint struct {
	Date         string `json:"date"`
	Requests     int    `json:"requests"`
	FailedReq    int    `json:"failed_requests"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// ModelDistributionPoint holds aggregated usage data for a single model.
type ModelDistributionPoint struct {
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

// QueryDailySeries returns per-day aggregated request count and token usage for a given API key.
func QueryDailySeries(apiKeyID int64, days int) ([]DailySeriesPoint, error) {
	return GormQueryDailySeries(apiKeyID, days)
}

// QueryModelDistribution returns request count and token usage grouped by model for a given API key.
func QueryModelDistribution(apiKeyID int64, days int) ([]ModelDistributionPoint, error) {
	return GormQueryModelDistribution(apiKeyID, days)
}

// APIKeyDistributionPoint holds aggregated usage data for a single API key.
type APIKeyDistributionPoint struct {
	APIKeyID int64  `json:"api_key_id"`
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

// QueryAPIKeyDistribution returns request count and token usage grouped by api_key.
func QueryAPIKeyDistribution(days int) ([]APIKeyDistributionPoint, error) {
	return GormQueryAPIKeyDistribution(days)
}

// GetDBPath returns the file path of the database, or empty if not initialised.
func GetDBPath() string {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()
	return usageDBPath
}

// HourlyTokenPoint holds token usage per hour for the last N hours.
type HourlyTokenPoint struct {
	Hour            string `json:"hour"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
}

// HourlyModelPoint holds model request counts per hour.
type HourlyModelPoint struct {
	Hour     string `json:"hour"`
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
}

// QueryHourlySeries returns per-hour token and model aggregates for the last N hours.
func QueryHourlySeries(apiKeyID int64, hours int) ([]HourlyTokenPoint, []HourlyModelPoint, error) {
	return GormQueryHourlySeries(apiKeyID, hours)
}

// EntityStatPoint holds aggregated usage data for a single entity (source or auth_index).
type EntityStatPoint struct {
	EntityName  string  `json:"entity_name"`
	Requests    int64   `json:"requests"`
	Failed      int64   `json:"failed"`
	AvgLatency  float64 `json:"avg_latency"`
	TotalTokens int64   `json:"total_tokens"`
}

// QueryEntityStats returns aggregates grouped by a given column (e.g. "source" or "auth_index").
func QueryEntityStats(apiKeyID int64, days int, groupColumn string) ([]EntityStatPoint, error) {
	return GormQueryEntityStats(apiKeyID, days, groupColumn)
}

func QueryDailyCallsByAuthIndexes(authIndexes []string, days int) ([]DailyCountPoint, error) {
	return GormQueryDailyCallsByAuthIndexes(authIndexes, days)
}

func QueryHourlyCallsByAuthIndex(authIndex string, hours int) ([]HourlyCountPoint, error) {
	return GormQueryHourlyCallsByAuthIndex(authIndex, hours)
}

func QueryRequestCountByAuthIndexSince(authIndex string, since time.Time) (int64, error) {
	return GormQueryRequestCountByAuthIndexSince(authIndex, since)
}

// GetRequestLogStorageBytes returns the approximate bytes currently occupied by
// stored request/response bodies.
func GetRequestLogStorageBytes() (int64, error) {
	return GormGetRequestLogStorageBytes()
}

// ChannelLatency holds the average latency stats for a single channel (source).
type ChannelLatency struct {
	Source string  `json:"source"`
	Count  int64   `json:"count"`
	AvgMs  float64 `json:"avg_ms"`
}

// GetChannelAvgLatency returns average request latency grouped by source (channel)
// for the last N days.
func GetChannelAvgLatency(days int) ([]ChannelLatency, error) {
	return GormGetChannelAvgLatency(days)
}

// CountTodayByKey returns the number of requests made by the given API key today (project timezone).
func CountTodayByKey(apiKeyID int64) (int64, error) {
	return GormCountTodayByKey(apiKeyID)
}

// CountTotalByKey returns the total number of requests made by the given API key.
func CountTotalByKey(apiKeyID int64) (int64, error) {
	return GormCountTotalByKey(apiKeyID)
}

// QueryTotalCostByKey returns the total accumulated cost for a given API key ID.
func QueryTotalCostByKey(apiKeyID int64) (float64, error) {
	return GormQueryTotalCostByKey(apiKeyID)
}

// buildWhereClause constructs a WHERE clause from query params for GORM usage.
func buildWhereClause(params LogQueryParams) (string, []interface{}) {
	conditions := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)

	// Time range: days=1 means "today", days=7 means "last 7 days", etc.
	conditions = append(conditions, "timestamp >= ?")
	args = append(args, CutoffStartUTC(params.Days).Format(time.RFC3339))

	if params.APIKeyID != 0 {
		if params.APIKeyID == -1 {
			conditions = append(conditions, "api_key_id = 0")
		} else {
			conditions = append(conditions, "api_key_id = ?")
			args = append(args, params.APIKeyID)
		}
	}
	if params.UserID != 0 {
		conditions = append(conditions, "user_id = ?")
		args = append(args, params.UserID)
	}
	if params.Model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, params.Model)
	}
	if params.Status == "success" {
		conditions = append(conditions, "failed = 0")
	} else if params.Status == "failed" {
		conditions = append(conditions, "failed = 1")
	}
	if len(params.AuthIndexes) > 0 || len(params.ChannelNames) > 0 {
		filterConditions := make([]string, 0, 2)

		authPlaceholders := make([]string, 0, len(params.AuthIndexes))
		for _, idx := range params.AuthIndexes {
			trimmed := strings.TrimSpace(idx)
			if trimmed == "" {
				continue
			}
			authPlaceholders = append(authPlaceholders, "?")
			args = append(args, trimmed)
		}
		if len(authPlaceholders) > 0 {
			filterConditions = append(filterConditions, "(auth_index IN ("+strings.Join(authPlaceholders, ",")+") AND trim(coalesce(channel_name, '')) = '')")
		}

		channelPlaceholders := make([]string, 0, len(params.ChannelNames))
		for _, name := range params.ChannelNames {
			trimmed := strings.ToLower(strings.TrimSpace(name))
			if trimmed == "" {
				continue
			}
			channelPlaceholders = append(channelPlaceholders, "?")
			args = append(args, trimmed)
		}
		if len(channelPlaceholders) > 0 {
			filterConditions = append(filterConditions, "lower(trim(channel_name)) IN ("+strings.Join(channelPlaceholders, ",")+")")
		}

		if len(filterConditions) > 0 {
			conditions = append(conditions, "("+strings.Join(filterConditions, " OR ")+")")
		} else {
			conditions = append(conditions, "1 = 0")
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// parseStoredTime parses a stored timestamp string into a time.Time.
func parseStoredTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// CalculateCost computes the cost for a given model's token usage.
// This is a convenience wrapper that delegates to the pricing cache.
func CalculateCost(model string, inputTokens, outputTokens, cachedTokens int64) float64 {
	return calculateCostFromCache(model, inputTokens, outputTokens, cachedTokens)
}

// calculateCostFromCache computes the cost using the in-memory pricing cache.
// For token-based pricing it uses per-million-token rates; for per-call pricing
// it returns the fixed PricePerCall.
// When a cached price exists and cached tokens are a subset of input tokens
// (cached <= input), the cached portion is deducted from input and charged at
// the cached rate. When cached tokens exceed input (context cache from prior
// turns), input is charged at the full rate and cached tokens are billed
// separately. When no cached price is set, cached tokens are ignored.
func calculateCostFromCache(model string, inputTokens, outputTokens, cachedTokens int64) float64 {
	if cfg, ok := GetModelConfig(model); ok && cfg.PricingMode == "call" {
		return cfg.PricePerCall
	}
	pricing, ok := GetModelPricing(model)
	if !ok {
		return 0
	}
	if pricing.CachedPricePerMillion > 0 && cachedTokens > 0 {
		if cachedTokens <= inputTokens {
			// Cached tokens are a subset of input: deduct from input, charge at cached rate
			nonCachedInput := inputTokens - cachedTokens
			return pricing.InputPricePerMillion*float64(nonCachedInput)/1_000_000 +
				pricing.OutputPricePerMillion*float64(outputTokens)/1_000_000 +
				pricing.CachedPricePerMillion*float64(cachedTokens)/1_000_000
		}
		// Cached tokens exceed input (context cache): charge both separately
		return pricing.InputPricePerMillion*float64(inputTokens)/1_000_000 +
			pricing.OutputPricePerMillion*float64(outputTokens)/1_000_000 +
			pricing.CachedPricePerMillion*float64(cachedTokens)/1_000_000
	}
	return pricing.InputPricePerMillion*float64(inputTokens)/1_000_000 +
		pricing.OutputPricePerMillion*float64(outputTokens)/1_000_000
}

// sortDailyCountPoints sorts daily count points by date.
func sortDailyCountPoints(points []DailyCountPoint) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].Date < points[j].Date
	})
}
