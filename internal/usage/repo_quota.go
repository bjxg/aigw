package usage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- GORM QuotaSnapshot functions ---

// GormRecordDailyQuotaSnapshot records daily quota snapshots using GORM.
func GormRecordDailyQuotaSnapshot(authIndex, provider string, quotas map[string]*float64) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" || len(quotas) == 0 {
		return nil
	}
	provider = strings.TrimSpace(provider)
	now := time.Now()
	dateKey := localDayKeyAt(now)
	recordedAt := now.UTC().Format(time.RFC3339Nano)

	return gormDB.Transaction(func(tx *gorm.DB) error {
		for key, rawPercent := range quotas {
			quotaKey := strings.TrimSpace(key)
			if quotaKey == "" {
				continue
			}

			m := AuthFileQuotaSnapshot{
				DateKey:    dateKey,
				AuthIndex:  authIndex,
				QuotaKey:   quotaKey,
				Provider:   provider,
				RecordedAt: recordedAt,
			}
			if rawPercent != nil {
				percent := *rawPercent
				if percent < 0 {
					percent = 0
				}
				if percent > 100 {
					percent = 100
				}
				m.Percent = &percent
			}

			result := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "date_key"},
					{Name: "auth_index"},
					{Name: "quota_key"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"provider", "percent", "recorded_at",
				}),
			}).Create(&m)

			if result.Error != nil {
				return fmt.Errorf("usage: GORM quota snapshot upsert: %w", result.Error)
			}
		}

		// Prune old snapshots
		retentionCutoff := cutoffDayKey(7)
		if err := tx.Where("date_key < ?", retentionCutoff).Delete(&AuthFileQuotaSnapshot{}).Error; err != nil {
			return fmt.Errorf("usage: GORM quota snapshot prune: %w", err)
		}

		return nil
	})
}

// GormRecordQuotaSnapshotPoints records quota snapshot points using GORM.
func GormRecordQuotaSnapshotPoints(authIndex, provider string, points []QuotaSnapshotPoint) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil
	}

	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" || len(points) == 0 {
		return nil
	}
	provider = strings.TrimSpace(provider)
	now := time.Now()

	return gormDB.Transaction(func(tx *gorm.DB) error {
		for _, point := range points {
			quotaKey := strings.TrimSpace(point.QuotaKey)
			if quotaKey == "" {
				continue
			}
			quotaLabel := strings.TrimSpace(point.QuotaLabel)
			if quotaLabel == "" {
				quotaLabel = quotaKey
			}
			recordedAt := point.RecordedAt
			if recordedAt.IsZero() {
				recordedAt = now
			}
			pointProvider := strings.TrimSpace(point.Provider)
			if pointProvider == "" {
				pointProvider = provider
			}

			m := AuthFileQuotaSnapshotPoint{
				RecordedAt:    recordedAt.UTC().Format(time.RFC3339Nano),
				AuthIndex:     authIndex,
				Provider:      pointProvider,
				QuotaKey:      quotaKey,
				QuotaLabel:    quotaLabel,
				WindowSeconds: point.WindowSeconds,
			}
			if point.Percent != nil {
				percent := *point.Percent
				if percent < 0 {
					percent = 0
				}
				if percent > 100 {
					percent = 100
				}
				m.Percent = &percent
			}
			if point.ResetAt != nil && !point.ResetAt.IsZero() {
				resetAtStr := point.ResetAt.UTC().Format(time.RFC3339Nano)
				m.ResetAt = &resetAtStr
			}

			if err := tx.Create(&m).Error; err != nil {
				return fmt.Errorf("usage: GORM quota snapshot points insert: %w", err)
			}
		}

		// Prune old points
		retentionCutoff := now.AddDate(0, 0, -8).UTC().Format(time.RFC3339Nano)
		if err := tx.Where("recorded_at < ?", retentionCutoff).Delete(&AuthFileQuotaSnapshotPoint{}).Error; err != nil {
			return fmt.Errorf("usage: GORM quota snapshot points prune: %w", err)
		}

		return nil
	})
}

// GormQueryDailyQuotaByAuthIndexes queries daily quota snapshots by auth indexes using GORM.
func GormQueryDailyQuotaByAuthIndexes(authIndexes []string, quotaKey string, days int) ([]DailyQuotaPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return []DailyQuotaPoint{}, nil
	}
	if days < 1 {
		days = 7
	}
	if len(authIndexes) == 0 {
		return []DailyQuotaPoint{}, nil
	}
	quotaKey = strings.TrimSpace(quotaKey)
	if quotaKey == "" {
		return []DailyQuotaPoint{}, nil
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
		return []DailyQuotaPoint{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalized)), ",")
	args := make([]interface{}, 0, len(normalized)+2)
	args = append(args, cutoffDayKey(days), quotaKey)
	for _, idx := range normalized {
		args = append(args, idx)
	}

	q := fmt.Sprintf(`
		SELECT date_key, AVG(percent) AS avg_percent, COUNT(percent) AS samples
		FROM auth_file_quota_snapshots
		WHERE date_key >= ? AND quota_key = ? AND auth_index IN (%s) AND percent IS NOT NULL
		GROUP BY date_key
		ORDER BY date_key ASC
	`, placeholders)

	rows, err := gormDB.Raw(q, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("usage: GORM daily quota by auth indexes query: %w", err)
	}
	defer rows.Close()

	result := make([]DailyQuotaPoint, 0, days)
	for rows.Next() {
		var point DailyQuotaPoint
		var percent sql.NullFloat64
		if err := rows.Scan(&point.Date, &percent, &point.Samples); err != nil {
			return nil, fmt.Errorf("usage: GORM daily quota by auth indexes scan: %w", err)
		}
		if percent.Valid {
			v := percent.Float64
			point.Percent = &v
		}
		result = append(result, point)
	}
	return result, rows.Err()
}

// GormQueryQuotaSnapshotPoints queries quota snapshot points using GORM.
func GormQueryQuotaSnapshotPoints(authIndex string, start, end time.Time) ([]QuotaSnapshotPoint, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return []QuotaSnapshotPoint{}, nil
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return []QuotaSnapshotPoint{}, nil
	}
	if start.IsZero() {
		start = time.Now().AddDate(0, 0, -7)
	}
	if end.IsZero() {
		end = time.Now()
	}

	q := `SELECT recorded_at, auth_index, provider, quota_key, quota_label, percent, reset_at, window_seconds
		FROM auth_file_quota_snapshot_points
		WHERE auth_index = ? AND recorded_at >= ? AND recorded_at <= ?
		ORDER BY recorded_at ASC`

	rows, err := gormDB.Raw(q, authIndex, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)).Rows()
	if err != nil {
		return nil, fmt.Errorf("usage: GORM quota snapshot points query: %w", err)
	}
	defer rows.Close()

	var result []QuotaSnapshotPoint
	for rows.Next() {
		var (
			p             QuotaSnapshotPoint
			resetAtStr    sql.NullString
			percent       sql.NullFloat64
			recordedAtStr string
		)
		if err := rows.Scan(&recordedAtStr, &p.AuthIndex, &p.Provider, &p.QuotaKey, &p.QuotaLabel, &percent, &resetAtStr, &p.WindowSeconds); err != nil {
			return nil, fmt.Errorf("usage: GORM quota snapshot points scan: %w", err)
		}
		p.RecordedAt, _ = time.Parse(time.RFC3339Nano, recordedAtStr)
		if percent.Valid {
			v := percent.Float64
			p.Percent = &v
		}
		if resetAtStr.Valid && resetAtStr.String != "" {
			t, err := time.Parse(time.RFC3339Nano, resetAtStr.String)
			if err == nil {
				p.ResetAt = &t
			}
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// GormQueryQuotaSnapshotSeries queries quota snapshot series using GORM.
func GormQueryQuotaSnapshotSeries(authIndex string, start, end time.Time) ([]QuotaSnapshotSeries, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return []QuotaSnapshotSeries{}, nil
	}
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return []QuotaSnapshotSeries{}, nil
	}
	if start.IsZero() {
		start = time.Now().AddDate(0, 0, -7)
	}
	if end.IsZero() {
		end = time.Now()
	}

	q := `SELECT quota_key, quota_label, percent, recorded_at, reset_at, window_seconds
		FROM auth_file_quota_snapshot_points
		WHERE auth_index = ? AND recorded_at >= ? AND recorded_at <= ?
		ORDER BY quota_key ASC, recorded_at ASC`

	rows, err := gormDB.Raw(q, authIndex, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)).Rows()
	if err != nil {
		return nil, fmt.Errorf("usage: GORM quota snapshot series query: %w", err)
	}
	defer rows.Close()

	seriesMap := make(map[string]*QuotaSnapshotSeries)
	var order []string

	for rows.Next() {
		var (
			quotaKey      string
			quotaLabel    string
			recordedAtStr string
			percent       sql.NullFloat64
			resetAtStr    sql.NullString
			windowSeconds int64
		)
		if err := rows.Scan(&quotaKey, &quotaLabel, &percent, &recordedAtStr, &resetAtStr, &windowSeconds); err != nil {
			return nil, fmt.Errorf("usage: GORM quota snapshot series scan: %w", err)
		}

		s, ok := seriesMap[quotaKey]
		if !ok {
			s = &QuotaSnapshotSeries{
				QuotaKey:      quotaKey,
				QuotaLabel:    quotaLabel,
				WindowSeconds: windowSeconds,
			}
			seriesMap[quotaKey] = s
			order = append(order, quotaKey)
		}

		recordedAt, _ := time.Parse(time.RFC3339Nano, recordedAtStr)
		point := QuotaSnapshotSeriesPoint{
			Timestamp: recordedAt,
		}
		if percent.Valid {
			v := percent.Float64
			point.Percent = &v
		}
		if resetAtStr.Valid && resetAtStr.String != "" {
			t, err := time.Parse(time.RFC3339Nano, resetAtStr.String)
			if err == nil {
				point.ResetAt = &t
			}
		}
		s.Points = append(s.Points, point)
	}

	result := make([]QuotaSnapshotSeries, 0, len(order))
	for _, key := range order {
		result = append(result, *seriesMap[key])
	}
	return result, rows.Err()
}
