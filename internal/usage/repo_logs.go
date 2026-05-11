package usage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// gormLogRepo implements LogRepository using GORM.
type gormLogRepo struct {
	db *gorm.DB
}

// NewGormLogRepository creates a LogRepository backed by GORM.
func NewGormLogRepository(gormDB *gorm.DB) LogRepository {
	return &gormLogRepo{db: gormDB}
}

// getGormDB returns the current GORM instance.
// This is a package-level helper until all code is migrated off the global *sql.DB.
func getGormDB() *gorm.DB {
	return db.GetGORM()
}

// dateTruncSQL returns a SQL expression string for date truncation.
// For SQLite, it uses date()/strftime(); for PostgreSQL, date_trunc().
// This is simpler than using clause.Expr for cases where we need the SQL as a string.
func dateTruncSQL(col string, granularity string) string {
	gormDB := getGormDB()
	if gormDB != nil && db.IsSQLiteDB(gormDB) {
		switch granularity {
		case "day":
			return fmt.Sprintf("date(%s, 'localtime')", col)
		case "hour":
			return fmt.Sprintf("strftime('%%Y-%%m-%%d %%H:00', %s, 'localtime')", col)
		}
	}
	// PostgreSQL and others
	switch granularity {
	case "day":
		return fmt.Sprintf("date_trunc('day', %s)", col)
	case "hour":
		return fmt.Sprintf("date_trunc('hour', %s)", col)
	}
	return col
}

// --- Insert ---

func (r *gormLogRepo) Insert(ctx context.Context, reqLog *RequestLog, inputContent, outputContent, detailContent string) error {
	cost := CalculateCost(reqLog.Model, reqLog.InputTokens, reqLog.OutputTokens, reqLog.CachedTokens)
	reqLog.Cost = cost

	// Use a background context for the transaction so that request cancellation
	// does not abort an audit record that has already been selected for persistence.
	return r.db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(reqLog).Error; err != nil {
			return fmt.Errorf("usage: insert log: %w", err)
		}

		// Always store content (RequestLogStorageConfig has been removed).
		if inputContent != "" || outputContent != "" || detailContent != "" {
			inputCompressed, err := compressLogContent(inputContent)
			if err != nil {
				return fmt.Errorf("usage: compress input content: %w", err)
			}
			outputCompressed, err := compressLogContent(outputContent)
			if err != nil {
				return fmt.Errorf("usage: compress output content: %w", err)
			}
			detailCompressed, err := compressLogContent(detailContent)
			if err != nil {
				return fmt.Errorf("usage: compress detail content: %w", err)
			}

			content := &RequestLogContent{
				LogID:         reqLog.ID,
				Timestamp:     reqLog.Timestamp,
				Compression:   requestLogContentCompression,
				InputContent:  inputCompressed,
				OutputContent: outputCompressed,
				DetailContent: detailCompressed,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "log_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"timestamp", "compression", "input_content", "output_content", "detail_content"}),
			}).Create(content).Error; err != nil {
				return fmt.Errorf("usage: insert log content: %w", err)
			}
		}
		return nil
	})
}

// --- Query ---

func (r *gormLogRepo) Query(ctx context.Context, params LogQueryParams) (LogQueryResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Size < 1 {
		params.Size = 50
	}
	if params.Size > 500 {
		params.Size = 500
	}
	if params.Days < 1 {
		params.Days = 7
	}

	query := r.db.WithContext(ctx).Model(&RequestLog{})

	// Apply filters
	query = applyLogFilters(query, params)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return LogQueryResult{}, fmt.Errorf("usage: count query: %w", err)
	}

	// Fetch page with has_content subquery
	offset := (params.Page - 1) * params.Size
	var items []LogRow

	selectFields := `id, timestamp, api_key_id, api_key_name, user_id, model, source, channel_name, auth_index,
		failed, latency_ms, first_token_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, cost,
		(CASE WHEN EXISTS (SELECT 1 FROM request_log_content content WHERE content.log_id = request_logs.id)
			OR length(input_content) > 0 OR length(output_content) > 0 THEN 1 ELSE 0 END) as has_content`

	rows, err := query.Select(selectFields).
		Order("timestamp DESC").
		Limit(params.Size).
		Offset(offset).
		Rows()
	if err != nil {
		return LogQueryResult{}, fmt.Errorf("usage: query logs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row LogRow
		var failedInt, hasContentInt int
		if err := rows.Scan(
			&row.ID, &row.Timestamp, &row.APIKeyID, &row.APIKeyName, &row.UserID, &row.Model, &row.Source, &row.ChannelName,
			&row.AuthIndex, &failedInt, &row.LatencyMs, &row.FirstTokenMs,
			&row.InputTokens, &row.OutputTokens, &row.ReasoningTokens,
			&row.CachedTokens, &row.TotalTokens, &row.Cost, &hasContentInt,
		); err != nil {
			return LogQueryResult{}, fmt.Errorf("usage: scan row: %w", err)
		}
		row.Failed = failedInt != 0
		row.HasContent = hasContentInt != 0
		items = append(items, row)
	}

	if items == nil {
		items = make([]LogRow, 0)
	}

	return LogQueryResult{
		Items: items,
		Total: total,
		Page:  params.Page,
		Size:  params.Size,
	}, nil
}

// --- DeleteByAPIKey ---

func (r *gormLogRepo) DeleteByAPIKeyID(ctx context.Context, apiKeyID int64) (int64, error) {
	if apiKeyID <= 0 {
		return 0, fmt.Errorf("usage: invalid api_key_id")
	}

	// Delete associated content rows first
	r.db.WithContext(ctx).
		Where("log_id IN (?)", r.db.Model(&RequestLog{}).Select("id").Where("api_key_id = ?", apiKeyID)).
		Delete(&RequestLogContent{})

	result := r.db.WithContext(ctx).Where("api_key_id = ?", apiKeyID).Delete(&RequestLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("usage: delete logs by api_key_id: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Infof("usage: deleted %d request log(s) for api_key_id=%d", result.RowsAffected, apiKeyID)
	}
	return result.RowsAffected, nil
}

// --- QueryContent ---

func (r *gormLogRepo) QueryContent(ctx context.Context, id int64) (LogContentResult, error) {
	// Use Row().Scan() for BLOB columns since GORM's struct Scan
	// does not properly handle []byte/BLOB mapping.
	var result LogContentResult
	var compression string
	var inputCompressed, outputCompressed []byte
	row := r.db.WithContext(ctx).Raw(`
		SELECT logs.id, logs.model, content.compression, content.input_content, content.output_content
		FROM request_logs logs
		JOIN request_log_content content ON content.log_id = logs.id
		WHERE logs.id = ?
	`, id).Row()
	err := row.Scan(&result.ID, &result.Model, &compression, &inputCompressed, &outputCompressed)
	if err == nil {
		inputDecoded, err := decompressLogContent(compression, inputCompressed)
		if err != nil {
			return LogContentResult{}, err
		}
		outputDecoded, err := decompressLogContent(compression, outputCompressed)
		if err != nil {
			return LogContentResult{}, err
		}
		result.InputContent = inputDecoded
		result.OutputContent = outputDecoded
		return result, nil
	}

	return LogContentResult{}, fmt.Errorf("usage: query log content: not found (id=%d)", id)
}

// --- QueryContentPart ---

func (r *gormLogRepo) QueryContentPart(ctx context.Context, id int64, part string) (LogContentPartResult, error) {
	part, err := normalizeLogContentPart(part)
	if err != nil {
		return LogContentPartResult{}, err
	}

	column := "input_content"
	if part == "output" {
		column = "output_content"
	} else if part == "details" {
		column = "detail_content"
	}

	// Try compressed content first using Row().Scan() for BLOB handling
	var logID int64
	var model string
	var compression string
	var content []byte
	row := r.db.WithContext(ctx).Raw(fmt.Sprintf(`
		SELECT logs.id, logs.model, content.compression, content.%s
		FROM request_logs logs
		JOIN request_log_content content ON content.log_id = logs.id
		WHERE logs.id = ?
	`, column), id).Row()
	err = row.Scan(&logID, &model, &compression, &content)
	if err == nil && logID > 0 {
		decoded, err := decompressLogContent(compression, content)
		if err != nil {
			return LogContentPartResult{}, err
		}
		return LogContentPartResult{
			ID:      logID,
			Model:   model,
			Content: decoded,
			Part:    part,
		}, nil
	}

	if part == "details" {
		var reqLog RequestLog
		if err := r.db.WithContext(ctx).Select("id, model").First(&reqLog, id).Error; err != nil {
			return LogContentPartResult{}, fmt.Errorf("usage: query log content part: %w", err)
		}
		return LogContentPartResult{ID: reqLog.ID, Model: reqLog.Model, Part: part}, nil
	}

	return LogContentPartResult{}, fmt.Errorf("usage: query log content part: not found (id=%d)", id)
}

// --- QueryContentForKey ---

func (r *gormLogRepo) QueryContentForKey(ctx context.Context, id int64, apiKeyID int64) (LogContentResult, error) {
	var result LogContentResult
	var compression string
	var inputCompressed, outputCompressed []byte
	row := r.db.WithContext(ctx).Raw(`
		SELECT logs.id, logs.model, content.compression, content.input_content, content.output_content
		FROM request_logs logs
		JOIN request_log_content content ON content.log_id = logs.id
		WHERE logs.id = ? AND logs.api_key_id = ?
	`, id, apiKeyID).Row()
	err := row.Scan(&result.ID, &result.Model, &compression, &inputCompressed, &outputCompressed)
	if err == nil && result.ID > 0 {
		inputDecoded, err := decompressLogContent(compression, inputCompressed)
		if err != nil {
			return LogContentResult{}, err
		}
		outputDecoded, err := decompressLogContent(compression, outputCompressed)
		if err != nil {
			return LogContentResult{}, err
		}
		result.InputContent = inputDecoded
		result.OutputContent = outputDecoded
		return result, nil
	}

	return LogContentResult{}, fmt.Errorf("usage: query log content: not found (id=%d, api_key_id=%d)", id, apiKeyID)
}

// --- QueryContentPartForKey ---

func (r *gormLogRepo) QueryContentPartForKey(ctx context.Context, id int64, apiKeyID int64, part string) (LogContentPartResult, error) {
	part, err := normalizeLogContentPart(part)
	if err != nil {
		return LogContentPartResult{}, err
	}

	column := "input_content"
	if part == "output" {
		column = "output_content"
	} else if part == "details" {
		column = "detail_content"
	}

	var logID int64
	var model string
	var compression string
	var content []byte
	row := r.db.WithContext(ctx).Raw(fmt.Sprintf(`
		SELECT logs.id, logs.model, content.compression, content.%s
		FROM request_logs logs
		JOIN request_log_content content ON content.log_id = logs.id
		WHERE logs.id = ? AND logs.api_key_id = ?
	`, column), id, apiKeyID).Row()
	err = row.Scan(&logID, &model, &compression, &content)
	if err == nil && logID > 0 {
		decoded, err := decompressLogContent(compression, content)
		if err != nil {
			return LogContentPartResult{}, err
		}
		return LogContentPartResult{
			ID:      logID,
			Model:   model,
			Content: decoded,
			Part:    part,
		}, nil
	}

	// Fallback: legacy inline content
	if part == "details" {
		var reqLog RequestLog
		if err := r.db.WithContext(ctx).Select("id, model").Where("id = ? AND api_key_id = ?", id, apiKeyID).First(&reqLog).Error; err != nil {
			return LogContentPartResult{}, fmt.Errorf("usage: query log content part: %w", err)
		}
		return LogContentPartResult{ID: reqLog.ID, Model: reqLog.Model, Part: part}, nil
	}

	return LogContentPartResult{}, fmt.Errorf("usage: query log content part: not found (id=%d)", id)
}

// --- QueryStats ---

func (r *gormLogRepo) QueryStats(ctx context.Context, params LogQueryParams) (LogStats, error) {
	if params.Days < 1 {
		params.Days = 7
	}

	query := r.db.WithContext(ctx).Model(&RequestLog{})
	query = applyLogFilters(query, params)

	var result struct {
		Total       int64
		Success     int64
		TotalTokens int64
		TotalCost   float64
	}
	err := query.Select(
		"COUNT(*) as total, COALESCE(SUM(CASE WHEN failed = false THEN 1 ELSE 0 END),0) as success, COALESCE(SUM(total_tokens),0) as total_tokens, COALESCE(SUM(cost),0) as total_cost",
	).Scan(&result).Error
	if err != nil {
		return LogStats{}, fmt.Errorf("usage: stats query: %w", err)
	}

	var successRate float64
	if result.Total > 0 {
		successRate = float64(result.Success) / float64(result.Total) * 100
	}

	return LogStats{
		Total:       result.Total,
		SuccessRate: successRate,
		TotalTokens: result.TotalTokens,
		TotalCost:   result.TotalCost,
	}, nil
}

// --- QueryFilters ---

func (r *gormLogRepo) QueryFilters(ctx context.Context, days int) (FilterOptions, error) {
	if days < 1 {
		days = 7
	}

	cutoff := CutoffStartUTC(days)

	keys, err := r.queryAPIKeyFilterItemsGorm(ctx)
	if err != nil {
		return FilterOptions{}, err
	}
	users, err := r.queryUserFilterItemsGorm(ctx, cutoff)
	if err != nil {
		return FilterOptions{}, err
	}
	models, err := r.queryDistinctGorm(ctx, "model", cutoff)
	if err != nil {
		return FilterOptions{}, err
	}
	channels, err := r.queryDistinctGorm(ctx, "channel_name", cutoff)
	if err != nil {
		return FilterOptions{}, err
	}

	return FilterOptions{
		APIKeys:  keys,
		Users:    users,
		Models:   models,
		Channels: channels,
	}, nil
}

func (r *gormLogRepo) queryDistinctGorm(ctx context.Context, column string, cutoff time.Time) ([]string, error) {
	var results []string
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Distinct(column).
		Where("timestamp >= ?", cutoff).
		Where(fmt.Sprintf("%s != ''", column)).
		Pluck(column, &results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: distinct %s: %w", column, err)
	}
	return results, nil
}

func (r *gormLogRepo) queryAPIKeyFilterItemsGorm(ctx context.Context) ([]APIKeyFilterItem, error) {
	var results []APIKeyFilterItem
	err := r.db.WithContext(ctx).Model(&APIKey{}).
		Select("id, name").
		Order("created_at ASC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: query api_key filter items: %w", err)
	}
	if results == nil {
		results = make([]APIKeyFilterItem, 0)
	}
	return results, nil
}

func (r *gormLogRepo) queryUserFilterItemsGorm(ctx context.Context, cutoff time.Time) ([]UserFilterItem, error) {
	var results []UserFilterItem
	err := r.db.WithContext(ctx).
		Table("request_logs").
		Select("DISTINCT users.id, users.name").
		Joins("JOIN users ON users.id = request_logs.user_id").
		Where("request_logs.timestamp >= ? AND request_logs.user_id IS NOT NULL", cutoff).
		Order("users.name").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: query user filter items: %w", err)
	}
	if results == nil {
		results = make([]UserFilterItem, 0)
	}
	return results, nil
}

// --- QueryDashboardKPI ---

func (r *gormLogRepo) QueryDashboardKPI(ctx context.Context, days int) (DashboardKPI, error) {
	if days < 1 {
		days = 7
	}
	cutoff := CutoffStartUTC(days)

	var kpi DashboardKPI
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select(`COUNT(*) as total_requests,
			COALESCE(SUM(CASE WHEN failed = false THEN 1 ELSE 0 END), 0) as success_requests,
			COALESCE(SUM(CASE WHEN failed = true THEN 1 ELSE 0 END), 0) as failed_requests,
			COALESCE(SUM(input_tokens), 0) as input_tokens,
			COALESCE(SUM(output_tokens), 0) as output_tokens,
			COALESCE(SUM(reasoning_tokens), 0) as reasoning_tokens,
			COALESCE(SUM(cached_tokens), 0) as cached_tokens,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(cost), 0) as total_cost`).
		Where("timestamp >= ?", cutoff).
		Scan(&kpi).Error
	if err != nil {
		return DashboardKPI{}, fmt.Errorf("usage: dashboard KPI query: %w", err)
	}

	if kpi.TotalRequests > 0 {
		kpi.SuccessRate = float64(kpi.SuccessRequests) / float64(kpi.TotalRequests) * 100
	}
	return kpi, nil
}

// --- QueryDashboardTrends ---

func (r *gormLogRepo) QueryDashboardTrends(ctx context.Context, days int) (DashboardTrends, error) {
	if days < 1 {
		days = 7
	}

	loc := getUsageLocation()
	buckets := buildDashboardBuckets(days, loc)
	byKey := make(map[string]*dashboardBucket, len(buckets))
	for i := range buckets {
		byKey[buckets[i].key] = &buckets[i]
	}

	type trendRow struct {
		Timestamp   time.Time
		Failed      bool
		TotalTokens int64
	}
	var rows []trendRow
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("timestamp, failed, total_tokens").
		Where("timestamp >= ?", CutoffStartUTC(days)).
		Find(&rows).Error
	if err != nil {
		return DashboardTrends{}, fmt.Errorf("usage: query dashboard trends: %w", err)
	}

	for _, row := range rows {
		key := dashboardBucketKey(row.Timestamp.In(loc), days)
		bucket := byKey[key]
		if bucket == nil {
			continue
		}
		bucket.requests++
		bucket.totalToken += row.TotalTokens
		if row.Failed {
			bucket.failed++
		} else {
			bucket.success++
		}
	}

	throughputSeries, err := r.queryDashboardThroughputSeries(ctx, time.Now(), loc)
	if err != nil {
		return DashboardTrends{}, err
	}

	trends := dashboardTrendsFromBuckets(buckets)
	trends.ThroughputSeries = throughputSeries
	return trends, nil
}

func (r *gormLogRepo) queryDashboardThroughputSeries(ctx context.Context, now time.Time, loc *time.Location) ([]DashboardThroughputPoint, error) {
	if loc == nil {
		loc = time.Local
	}

	buckets := buildRecentThroughputBucketsAt(now, loc)
	byKey := make(map[string]*dashboardBucket, len(buckets))
	for i := range buckets {
		byKey[buckets[i].key] = &buckets[i]
	}

	start := now.In(loc).Truncate(time.Minute).Add(-time.Duration(dashboardThroughputBucketCount-1) * time.Minute)

	type throughputRow struct {
		Timestamp   time.Time
		TotalTokens int64
	}
	var rows []throughputRow
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("timestamp, total_tokens").
		Where("timestamp >= ?", start.UTC()).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("usage: query dashboard throughput trends: %w", err)
	}

	for _, row := range rows {
		key := row.Timestamp.In(loc).Truncate(time.Minute).Format("2006-01-02 15:04")
		bucket := byKey[key]
		if bucket == nil {
			continue
		}
		bucket.requests++
		bucket.totalToken += row.TotalTokens
	}

	return throughputSeriesFromBuckets(buckets), nil
}

// --- QueryDailySeries ---

func (r *gormLogRepo) QueryDailySeries(ctx context.Context, apiKeyID int64, days int) ([]DailySeriesPoint, error) {
	if days < 1 {
		days = 7
	}

	query := r.db.WithContext(ctx).Model(&RequestLog{})
	query = applyTimeAndAPIKeyFilter(query, apiKeyID, days)

	// Use dialect-specific date truncation expression
	dateExpr := dateTruncSQL("timestamp", "day")

	var results []DailySeriesPoint
	err := query.Select(dateExpr + " as date, COUNT(*) as requests, SUM(CASE WHEN failed = true THEN 1 ELSE 0 END) as failed_requests, COALESCE(SUM(input_tokens),0) as input_tokens, COALESCE(SUM(output_tokens),0) as output_tokens").
		Group("date").
		Order("date").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: daily series query: %w", err)
	}
	return results, nil
}

// --- QueryModelDistribution ---

func (r *gormLogRepo) QueryModelDistribution(ctx context.Context, apiKeyID int64, days int) ([]ModelDistributionPoint, error) {
	if days < 1 {
		days = 7
	}

	query := r.db.WithContext(ctx).Model(&RequestLog{})
	query = applyTimeAndAPIKeyFilter(query, apiKeyID, days)

	var results []ModelDistributionPoint
	err := query.Select("model, COUNT(*) as requests, COALESCE(SUM(total_tokens),0) as tokens").
		Where("model != ''").
		Group("model").
		Order("requests DESC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: model distribution query: %w", err)
	}
	return results, nil
}

// --- QueryAPIKeyDistribution ---

func (r *gormLogRepo) QueryAPIKeyDistribution(ctx context.Context, days int) ([]APIKeyDistributionPoint, error) {
	if days < 1 {
		days = 7
	}

	cutoff := CutoffStartUTC(days)
	var results []APIKeyDistributionPoint
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("api_key_id, COALESCE(NULLIF(MAX(api_key_name),''), k.name, '') as name, COUNT(*) as requests, COALESCE(SUM(total_tokens),0) as tokens").
		Joins("LEFT JOIN api_keys k ON k.id = api_key_id").
		Where("timestamp >= ? AND api_key_id != 0", cutoff).
		Group("api_key_id").
		Order("requests DESC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: apikey distribution query: %w", err)
	}
	return results, nil
}

// --- QueryHourlySeries ---

func (r *gormLogRepo) QueryHourlySeries(ctx context.Context, apiKeyID int64, hours int) ([]HourlyTokenPoint, []HourlyModelPoint, error) {
	if hours < 1 {
		hours = 24
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC()

	query := r.db.WithContext(ctx).Model(&RequestLog{}).Where("timestamp >= ?", cutoff)
	if apiKeyID != 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}

	// Token aggregation by hour
	hourExpr := dateTruncSQL("timestamp", "hour")
	var tokens []HourlyTokenPoint
	err := query.Select(hourExpr+" as hour, COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(reasoning_tokens),0), COALESCE(SUM(cached_tokens),0), COALESCE(SUM(total_tokens),0)").
		Group("hour").
		Order("hour").
		Find(&tokens).Error
	if err != nil {
		return nil, nil, fmt.Errorf("usage: hourly token query: %w", err)
	}

	// Model counts by hour
	var models []HourlyModelPoint
	err = query.Select(hourExpr+" as hour, model, COUNT(*) as requests").
		Where("model != ''").
		Group("hour, model").
		Order("hour").
		Find(&models).Error
	if err != nil {
		return nil, nil, fmt.Errorf("usage: hourly model query: %w", err)
	}

	return tokens, models, nil
}

// --- QueryEntityStats ---

func (r *gormLogRepo) QueryEntityStats(ctx context.Context, apiKeyID int64, days int, groupColumn string) ([]EntityStatPoint, error) {
	if days < 1 {
		days = 7
	}
	if groupColumn != "source" && groupColumn != "auth_index" {
		return nil, fmt.Errorf("usage: invalid group column")
	}

	query := r.db.WithContext(ctx).Model(&RequestLog{})
	query = applyTimeAndAPIKeyFilter(query, apiKeyID, days)

	var results []EntityStatPoint
	err := query.Select(fmt.Sprintf("%s as entity_name, COUNT(*) as requests, COALESCE(SUM(CASE WHEN failed = true THEN 1 ELSE 0 END),0) as failed, COALESCE(AVG(latency_ms),0) as avg_latency, COALESCE(SUM(total_tokens),0) as total_tokens", groupColumn)).
		Where(fmt.Sprintf("%s != ''", groupColumn)).
		Group(groupColumn).
		Order("requests DESC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: entity stats query: %w", err)
	}
	return results, nil
}

// --- QueryLogStorageBytes ---

func (r *gormLogRepo) QueryLogStorageBytes(ctx context.Context) (int64, error) {
	// Count bytes from request_log_content (compressed)
	var contentBytes int64
	err := r.db.WithContext(ctx).Model(&RequestLogContent{}).
		Select("COALESCE(SUM(LENGTH(input_content) + LENGTH(output_content) + LENGTH(detail_content)), 0)").
		Scan(&contentBytes).Error
	if err != nil {
		return 0, fmt.Errorf("usage: query request log storage bytes: %w", err)
	}

	// Count bytes from legacy inline content in request_logs
	var legacyBytes int64
	err = r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("COALESCE(SUM(CASE WHEN LENGTH(input_content) > 0 OR LENGTH(output_content) > 0 THEN LENGTH(input_content) + LENGTH(output_content) ELSE 0 END), 0)").
		Where("LENGTH(input_content) > 0 OR LENGTH(output_content) > 0").
		Scan(&legacyBytes).Error
	if err != nil {
		// Non-fatal: legacy content may not exist
		return contentBytes, nil
	}

	return contentBytes + legacyBytes, nil
}

// --- GetChannelAvgLatency ---

func (r *gormLogRepo) GetChannelAvgLatency(ctx context.Context, days int) ([]ChannelLatency, error) {
	if days < 1 {
		days = 7
	}
	cutoff := CutoffStartUTC(days)

	var results []ChannelLatency
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("source as source_key, COUNT(*) as count, AVG(latency_ms) as avg_ms").
		Where("timestamp > ? AND source != ''", cutoff).
		Group("source").
		Order("avg_ms DESC").
		Limit(5).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: query channel latency: %w", err)
	}
	return results, nil
}

// --- CountTodayByKey ---

func (r *gormLogRepo) CountTodayByKey(ctx context.Context, apiKeyID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Where("api_key_id = ? AND timestamp >= ?", apiKeyID, CutoffStartUTC(1)).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("usage: count today: %w", err)
	}
	return count, nil
}

// --- CountTotalByKey ---

func (r *gormLogRepo) CountTotalByKey(ctx context.Context, apiKeyID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Where("api_key_id = ?", apiKeyID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("usage: count total: %w", err)
	}
	return count, nil
}

// --- QueryTotalCostByKey ---

func (r *gormLogRepo) QueryTotalCostByKey(ctx context.Context, apiKeyID int64) (float64, error) {
	var total float64
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Select("COALESCE(SUM(cost), 0)").
		Where("api_key_id = ?", apiKeyID).
		Scan(&total).Error
	if err != nil {
		return 0, fmt.Errorf("usage: query total cost: %w", err)
	}
	return total, nil
}

// --- QueryModelsForKey ---

func (r *gormLogRepo) QueryModelsForKey(ctx context.Context, apiKeyID int64, days int) ([]string, error) {
	if days < 1 {
		days = 7
	}
	cutoff := CutoffStartUTC(days)

	var results []string
	err := r.db.WithContext(ctx).Model(&RequestLog{}).
		Distinct("model").
		Where("api_key_id = ? AND timestamp >= ? AND model != ''", apiKeyID, cutoff).
		Pluck("model", &results).Error
	if err != nil {
		return nil, fmt.Errorf("usage: distinct models for key: %w", err)
	}
	return results, nil
}

// --- Helper functions for GORM filters ---

// applyLogFilters applies common filter parameters to a GORM query.
func applyLogFilters(query *gorm.DB, params LogQueryParams) *gorm.DB {
	cutoff := CutoffStartUTC(params.Days)
	query = query.Where("timestamp >= ?", cutoff)

	if params.APIKeyID != 0 {
		if params.APIKeyID == -1 {
			// System requests: api_key_id = 0
			query = query.Where("api_key_id = 0")
		} else {
			query = query.Where("api_key_id = ?", params.APIKeyID)
		}
	}

	if params.UserID != 0 {
		query = query.Where("user_id = ?", params.UserID)
	}

	if params.Model != "" {
		query = query.Where("model = ?", params.Model)
	}

	if params.Status == "success" {
		query = query.Where("failed = false")
	} else if params.Status == "failed" {
		query = query.Where("failed = true")
	}

	if len(params.AuthIndexes) > 0 || len(params.ChannelNames) > 0 {
		filterConditions := buildGormFilterConditionOr(query, params)

		if len(filterConditions) > 0 {
			query = query.Where(strings.Join(filterConditions, " OR "))
		} else {
			query = query.Where("1 = 0")
		}
	}

	return query
}

// applyTimeAndAPIKeyFilter applies a simple time range + optional API key ID filter.
func applyTimeAndAPIKeyFilter(query *gorm.DB, apiKeyID int64, days int) *gorm.DB {
	cutoff := CutoffStartUTC(days)
	query = query.Where("timestamp >= ?", cutoff)
	if apiKeyID != 0 {
		query = query.Where("api_key_id = ?", apiKeyID)
	}
	return query
}

// buildGormFilterConditionOr builds OR conditions for auth_indexes and channel_names.
// It applies the IN clauses directly to the query and returns the SQL condition strings.
func buildGormFilterConditionOr(query *gorm.DB, params LogQueryParams) []string {
	var filterConditions []string

	if len(params.AuthIndexes) > 0 {
		filterConditions = append(filterConditions, "(auth_index IN ? AND trim(coalesce(channel_name, '')) = '')")
	}

	if len(params.ChannelNames) > 0 {
		filterConditions = append(filterConditions, "lower(trim(channel_name)) IN ?")
	}

	return filterConditions
}

// --- GORM-based implementations of package-level functions ---

// GormInsertLog is the GORM-backed implementation of the package-level InsertLog.
// It writes a single request log entry and its content.
func GormInsertLog(apiKeyID int64, apiKeyName string, userID *int64, model, source, channelName, authIndex string,
	failed bool, timestamp time.Time, latencyMs, firstTokenMs int64, tokens TokenStats,
	inputContent, outputContent, detailContent string) {

	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	reqLog := &RequestLog{
		Timestamp:       timestamp.UTC(),
		APIKeyID:        apiKeyID,
		APIKeyName:      apiKeyName,
		UserID:          userID,
		Model:           model,
		Source:          source,
		ChannelName:     channelName,
		AuthIndex:       authIndex,
		Failed:          failed,
		LatencyMs:       latencyMs,
		FirstTokenMs:    firstTokenMs,
		InputTokens:     tokens.InputTokens,
		OutputTokens:    tokens.OutputTokens,
		ReasoningTokens: tokens.ReasoningTokens,
		CachedTokens:    tokens.CachedTokens,
		TotalTokens:     tokens.TotalTokens,
	}

	repo := &gormLogRepo{db: gormDB}
	if err := repo.Insert(context.Background(), reqLog, inputContent, outputContent, detailContent); err != nil {
		log.Errorf("usage: insert log via GORM: %v", err)
		return
	}

	// Notify TPM tracker about token usage (keep string callback for memory-level RPM/TPM tracking)
	if tokenUsageCallback != nil && tokens.TotalTokens > 0 {
		tokenUsageCallback(apiKeyName, tokens.TotalTokens)
	}
}

// GormQueryLogs is the GORM-backed implementation of QueryLogs.
func GormQueryLogs(params LogQueryParams) (LogQueryResult, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogQueryResult{
			Items: make([]LogRow, 0),
			Total: 0,
			Page:  params.Page,
			Size:  params.Size,
		}, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.Query(context.Background(), params)
}

// GormQueryFilters is the GORM-backed implementation of QueryFilters.
func GormQueryFilters(days int) (FilterOptions, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return FilterOptions{
			APIKeys:  make([]APIKeyFilterItem, 0),
			Users:    make([]UserFilterItem, 0),
			Models:   make([]string, 0),
			Channels: make([]string, 0),
		}, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryFilters(context.Background(), days)
}

// GormQueryStats is the GORM-backed implementation of QueryStats.
func GormQueryStats(params LogQueryParams) (LogStats, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogStats{}, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryStats(context.Background(), params)
}

// GormDeleteLogsByAPIKeyID is the GORM-backed implementation of DeleteLogsByAPIKeyID.
func GormDeleteLogsByAPIKeyID(apiKeyID int64) (int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.DeleteByAPIKeyID(context.Background(), apiKeyID)
}

// GormQueryLogContent is the GORM-backed implementation of QueryLogContent.
func GormQueryLogContent(id int64) (LogContentResult, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogContentResult{}, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryContent(context.Background(), id)
}

// GormQueryLogContentPart is the GORM-backed implementation of QueryLogContentPart.
func GormQueryLogContentPart(id int64, part string) (LogContentPartResult, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogContentPartResult{}, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryContentPart(context.Background(), id, part)
}

// GormQueryLogContentForKey is the GORM-backed implementation of QueryLogContentForKey.
func GormQueryLogContentForKey(id int64, apiKeyID int64) (LogContentResult, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogContentResult{}, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryContentForKey(context.Background(), id, apiKeyID)
}

// GormQueryLogContentPartForKey is the GORM-backed implementation of QueryLogContentPartForKey.
func GormQueryLogContentPartForKey(id int64, apiKeyID int64, part string) (LogContentPartResult, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return LogContentPartResult{}, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryContentPartForKey(context.Background(), id, apiKeyID, part)
}

// GormQueryDashboardKPI is the GORM-backed implementation of QueryDashboardKPI.
func GormQueryDashboardKPI(days int) (DashboardKPI, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return DashboardKPI{}, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryDashboardKPI(context.Background(), days)
}

// GormQueryDashboardTrends is the GORM-backed implementation of QueryDashboardTrends.
func GormQueryDashboardTrends(days int) (DashboardTrends, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return emptyDashboardTrends(days), nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryDashboardTrends(context.Background(), days)
}

// GormQueryDailySeries is the GORM-backed implementation of QueryDailySeries.
func GormQueryDailySeries(apiKeyID int64, days int) ([]DailySeriesPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryDailySeries(context.Background(), apiKeyID, days)
}

// GormQueryModelDistribution is the GORM-backed implementation of QueryModelDistribution.
func GormQueryModelDistribution(apiKeyID int64, days int) ([]ModelDistributionPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryModelDistribution(context.Background(), apiKeyID, days)
}

// GormQueryAPIKeyDistribution is the GORM-backed implementation of QueryAPIKeyDistribution.
func GormQueryAPIKeyDistribution(days int) ([]APIKeyDistributionPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryAPIKeyDistribution(context.Background(), days)
}

// GormQueryHourlySeries is the GORM-backed implementation of QueryHourlySeries.
func GormQueryHourlySeries(apiKeyID int64, hours int) ([]HourlyTokenPoint, []HourlyModelPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, nil, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryHourlySeries(context.Background(), apiKeyID, hours)
}

// GormQueryEntityStats is the GORM-backed implementation of QueryEntityStats.
func GormQueryEntityStats(apiKeyID int64, days int, groupColumn string) ([]EntityStatPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryEntityStats(context.Background(), apiKeyID, days, groupColumn)
}

// GormGetRequestLogStorageBytes is the GORM-backed implementation of GetRequestLogStorageBytes.
func GormGetRequestLogStorageBytes() (int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryLogStorageBytes(context.Background())
}

// GormGetChannelAvgLatency is the GORM-backed implementation of GetChannelAvgLatency.
func GormGetChannelAvgLatency(days int) ([]ChannelLatency, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, fmt.Errorf("usage: database not initialised")
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.GetChannelAvgLatency(context.Background(), days)
}

// GormCountTodayByKey is the GORM-backed implementation of CountTodayByKey.
func GormCountTodayByKey(apiKeyID int64) (int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.CountTodayByKey(context.Background(), apiKeyID)
}

// GormCountTotalByKey is the GORM-backed implementation of CountTotalByKey.
func GormCountTotalByKey(apiKeyID int64) (int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.CountTotalByKey(context.Background(), apiKeyID)
}

// GormQueryModelsForKey is the GORM-backed implementation of QueryModelsForKey.
func GormQueryModelsForKey(apiKeyID int64, days int) ([]string, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return make([]string, 0), nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryModelsForKey(context.Background(), apiKeyID, days)
}

// GormQueryTotalCostByKey is the GORM-backed implementation of QueryTotalCostByKey.
func GormQueryTotalCostByKey(apiKeyID int64) (float64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, nil
	}
	repo := &gormLogRepo{db: gormDB}
	return repo.QueryTotalCostByKey(context.Background(), apiKeyID)
}

// GormQueryDailyCallsByAuthIndexes is the GORM-backed implementation of QueryDailyCallsByAuthIndexes.
func GormQueryDailyCallsByAuthIndexes(authIndexes []string, days int) ([]DailyCountPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return []DailyCountPoint{}, nil
	}
	if days < 1 {
		days = 7
	}
	if len(authIndexes) == 0 {
		return []DailyCountPoint{}, nil
	}

	seen := make(map[string]struct{}, len(authIndexes))
	normalized := make([]string, 0, len(authIndexes))
	for _, idx := range authIndexes {
		idx = strings.TrimSpace(idx)
		if idx == "" {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		normalized = append(normalized, idx)
	}
	if len(normalized) == 0 {
		return []DailyCountPoint{}, nil
	}

	cutoff := CutoffStartUTC(days)

	type rowResult struct {
		Timestamp time.Time
	}
	var rows []rowResult
	err := gormDB.Model(&RequestLog{}).
		Select("timestamp").
		Where("timestamp >= ? AND auth_index IN ?", cutoff, normalized).
		Order("timestamp ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("usage: daily calls by auth indexes query: %w", err)
	}

	byDate := make(map[string]int64, days)
	for _, row := range rows {
		byDate[localDayKeyAt(row.Timestamp)]++
	}

	result := make([]DailyCountPoint, 0, len(byDate))
	for date, requests := range byDate {
		result = append(result, DailyCountPoint{Date: date, Requests: requests})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})
	return result, nil
}

// GormQueryHourlyCallsByAuthIndex is the GORM-backed implementation of QueryHourlyCallsByAuthIndex.
func GormQueryHourlyCallsByAuthIndex(authIndex string, hours int) ([]HourlyCountPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return []HourlyCountPoint{}, nil
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return []HourlyCountPoint{}, nil
	}
	if hours < 1 {
		hours = 5
	}
	if hours > 24 {
		hours = 24
	}

	loc := getUsageLocation()
	now := time.Now().In(loc).Truncate(time.Hour)
	start := now.Add(-time.Duration(hours-1) * time.Hour)
	buckets := make([]HourlyCountPoint, 0, hours)
	byKey := make(map[string]*HourlyCountPoint, hours)
	for i := 0; i < hours; i++ {
		key := start.Add(time.Duration(i) * time.Hour).Format("2006-01-02 15:00")
		buckets = append(buckets, HourlyCountPoint{Hour: key, Requests: 0})
		byKey[key] = &buckets[len(buckets)-1]
	}

	type rowResult struct {
		Timestamp time.Time
	}
	var rows []rowResult
	err := gormDB.Model(&RequestLog{}).
		Select("timestamp").
		Where("timestamp >= ? AND auth_index = ?", start.UTC(), authIndex).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("usage: hourly calls by auth index query: %w", err)
	}

	for _, row := range rows {
		key := row.Timestamp.In(loc).Truncate(time.Hour).Format("2006-01-02 15:00")
		if bucket := byKey[key]; bucket != nil {
			bucket.Requests++
		}
	}
	return buckets, nil
}

// GormQueryRequestCountByAuthIndexSince is the GORM-backed implementation of QueryRequestCountByAuthIndexSince.
func GormQueryRequestCountByAuthIndexSince(authIndex string, since time.Time) (int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return 0, nil
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return 0, nil
	}
	var count int64
	err := gormDB.Model(&RequestLog{}).
		Where("timestamp >= ? AND auth_index = ?", since.UTC(), authIndex).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("usage: request count by auth index query: %w", err)
	}
	return count, nil
}
