package usage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/db"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const requestLogContentCompression = "zstd"

const (
	// Avoid vacuuming too frequently; VACUUM can be expensive on large DBs.
	sqliteVacuumMinInterval = 2 * time.Hour

	// Only vacuum when there's enough reclaimable space to matter.
	sqliteVacuumMinReclaimBytes = 64 << 20 // 64 MiB

	// If reclaimable bytes are smaller, require a higher ratio to vacuum.
	sqliteVacuumMinReclaimRatio = 0.20
)

type requestLogStorageRuntime struct {
	StoreContent           bool
	ContentRetentionDays   int
	CleanupIntervalMinutes int
	MaxTotalSizeMB         int
	VacuumOnCleanup        bool
}

var (
	requestLogStorage = requestLogStorageRuntime{
		StoreContent:           true,
		ContentRetentionDays:   30,
		CleanupIntervalMinutes: 1440,
		MaxTotalSizeMB:         1024,
		VacuumOnCleanup:        true,
	}

	requestLogMaintenanceCancel context.CancelFunc
	requestLogMaintenanceWG     sync.WaitGroup
	requestLogMaintenanceWakeup atomic.Value // chan struct{}

	lastUsageVacuumUnixNano atomic.Int64
	requestLogContentBytes  atomic.Int64 // total compressed bytes; -1 means unknown

	zstdEncoderPool = sync.Pool{
		New: func() any {
			encoder, err := zstd.NewWriter(nil)
			if err != nil {
				panic(err)
			}
			return encoder
		},
	}
	zstdDecoderPool = sync.Pool{
		New: func() any {
			decoder, err := zstd.NewReader(nil)
			if err != nil {
				panic(err)
			}
			return decoder
		},
	}
)

func init() {
	requestLogContentBytes.Store(-1)
	// Initialize atomic.Value type so subsequent stores can use typed nil safely.
	requestLogMaintenanceWakeup.Store((chan struct{})(nil))
}

func contentRetentionUnlimited() bool {
	return requestLogStorage.ContentRetentionDays <= 0
}

func normalizeRequestLogStorageConfig(cfg config.RequestLogStorageConfig) requestLogStorageRuntime {
	if !cfg.StoreContent && cfg.ContentRetentionDays == 0 && cfg.CleanupIntervalMinutes == 0 && !cfg.VacuumOnCleanup {
		return requestLogStorageRuntime{
			StoreContent:           true,
			ContentRetentionDays:   30,
			CleanupIntervalMinutes: 1440,
			MaxTotalSizeMB:         1024,
			VacuumOnCleanup:        true,
		}
	}

	runtimeCfg := requestLogStorageRuntime{
		StoreContent:           cfg.StoreContent,
		ContentRetentionDays:   cfg.ContentRetentionDays,
		CleanupIntervalMinutes: cfg.CleanupIntervalMinutes,
		MaxTotalSizeMB:         cfg.MaxTotalSizeMB,
		VacuumOnCleanup:        cfg.VacuumOnCleanup,
	}
	if runtimeCfg.ContentRetentionDays < 0 {
		runtimeCfg.ContentRetentionDays = 0
	}
	if runtimeCfg.CleanupIntervalMinutes <= 0 {
		runtimeCfg.CleanupIntervalMinutes = 1440
	}
	if runtimeCfg.MaxTotalSizeMB < 0 {
		runtimeCfg.MaxTotalSizeMB = 0
	}
	return runtimeCfg
}

func maxLogContentBytes() int64 {
	if requestLogStorage.MaxTotalSizeMB <= 0 {
		return 0
	}
	return int64(requestLogStorage.MaxTotalSizeMB) * 1024 * 1024
}

func requestLogMaintenanceWakeupChan() chan struct{} {
	value := requestLogMaintenanceWakeup.Load()
	if value == nil {
		return nil
	}
	ch, _ := value.(chan struct{})
	return ch
}

func triggerRequestLogCompaction() {
	ch := requestLogMaintenanceWakeupChan()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func startRequestLogMaintenance() {
	stopRequestLogMaintenance()
	if !requestLogStorage.StoreContent {
		return
	}

	gormDB := getGormDB()
	if gormDB == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	requestLogMaintenanceCancel = cancel
	wakeup := make(chan struct{}, 1)
	requestLogMaintenanceWakeup.Store(wakeup)
	requestLogMaintenanceWG.Add(1)
	// 请求日志维护协程属于 usage 存储子系统：
	// - owner: startRequestLogMaintenance / stopRequestLogMaintenance
	// - 取消条件: stopRequestLogMaintenance、数据库关闭、进程退出
	// - 超时策略: 周期 cleanup + wakeup 驱动；单次 DB 操作各自控制
	// - 清理方式: cancel 后等待 requestLogMaintenanceWG，确保协程退出
	go func() {
		defer requestLogMaintenanceWG.Done()
		runRequestLogMaintenancePass(gormDB)

		ticker := time.NewTicker(time.Duration(requestLogStorage.CleanupIntervalMinutes) * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-wakeup:
				// Compaction wakeup (triggered by size-cap pruning during inserts).
				// Only run SQLite-specific compaction when using SQLite.
				if db.IsSQLite() {
					runSQLiteCompaction(gormDB, false)
				}
			case <-ticker.C:
				runRequestLogMaintenancePass(gormDB)
			}
		}
	}()
}

func stopRequestLogMaintenance() {
	if requestLogMaintenanceCancel != nil {
		requestLogMaintenanceCancel()
		requestLogMaintenanceWG.Wait()
		requestLogMaintenanceCancel = nil
	}
	requestLogMaintenanceWakeup.Store((chan struct{})(nil))
}

func runRequestLogMaintenancePass(gormDB *gorm.DB) {
	if gormDB == nil {
		return
	}

	// Refresh the running total periodically so size-cap enforcement stays fast
	// and accurate without per-request full table scans.
	if requestLogContentBytes.Load() < 0 {
		if total, err := queryStoredContentBytes(gormDB); err == nil {
			requestLogContentBytes.Store(total)
		}
	}

	deleted, err := cleanupExpiredLogContent(gormDB)
	if err != nil {
		log.Errorf("usage: cleanup request log content: %v", err)
		return
	}
	if deleted > 0 {
		log.Infof("usage: pruned %d expired request log content rows", deleted)
	}

	trimmed, err := cleanupOversizedLogContent(gormDB, maxLogContentBytes())
	if err != nil {
		log.Errorf("usage: enforce request log content size cap: %v", err)
		return
	}
	if trimmed > 0 {
		log.Infof("usage: pruned %d request log content rows to enforce size cap", trimmed)
	}

	// After maintenance changes, refresh the exact total once to keep the running
	// counter accurate (avoids drift from pruning/migration deletes).
	if total, err := queryStoredContentBytes(gormDB); err == nil {
		requestLogContentBytes.Store(total)
	} else {
		requestLogContentBytes.Store(-1)
	}

	// SQLite-specific compaction: checkpoint + conditional vacuum.
	// Only runs when the database is SQLite.
	if db.IsSQLite() {
		runSQLiteCompaction(gormDB, true)
	}
}

func compressLogContent(content string) ([]byte, error) {
	if content == "" {
		return []byte{}, nil
	}
	encoder := zstdEncoderPool.Get().(*zstd.Encoder)
	defer zstdEncoderPool.Put(encoder)
	return encoder.EncodeAll([]byte(content), make([]byte, 0, len(content)/2)), nil
}

func decompressLogContent(compression string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", nil
	}
	switch compression {
	case "", requestLogContentCompression:
		decoder := zstdDecoderPool.Get().(*zstd.Decoder)
		defer zstdDecoderPool.Put(decoder)
		decoded, err := decoder.DecodeAll(content, nil)
		if err != nil {
			return "", fmt.Errorf("usage: decompress content: %w", err)
		}
		return string(decoded), nil
	default:
		return "", fmt.Errorf("usage: unsupported content compression %q", compression)
	}
}

func withinContentRetention(timestamp time.Time) bool {
	if contentRetentionUnlimited() {
		return true
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -requestLogStorage.ContentRetentionDays)
	return !timestamp.Before(cutoff)
}

func cleanupExpiredLogContent(gormDB *gorm.DB) (int64, error) {
	if gormDB == nil || contentRetentionUnlimited() {
		return 0, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -requestLogStorage.ContentRetentionDays)

	// Delete expired content rows
	contentResult := gormDB.Where("timestamp < ?", cutoff).Delete(&RequestLogContent{})
	if contentResult.Error != nil {
		return 0, fmt.Errorf("usage: delete expired content: %w", contentResult.Error)
	}

	// Clear legacy inline content for expired rows
	legacyResult := gormDB.Model(&RequestLog{}).
		Where("timestamp < ? AND (length(input_content) > 0 OR length(output_content) > 0)", cutoff).
		UpdateColumns(map[string]interface{}{
			"input_content":  "",
			"output_content": "",
		})
	if legacyResult.Error != nil {
		return 0, fmt.Errorf("usage: clear expired legacy content: %w", legacyResult.Error)
	}

	totalChanged := contentResult.RowsAffected + legacyResult.RowsAffected
	if totalChanged == 0 {
		return 0, nil
	}
	return totalChanged, nil
}

func cleanupOversizedLogContent(gormDB *gorm.DB, maxBytes int64) (int64, error) {
	if gormDB == nil {
		return 0, nil
	}
	if maxBytes <= 0 {
		return 0, nil
	}

	totalBytes, err := queryStoredContentBytes(gormDB)
	if err != nil {
		return 0, err
	}

	var deletedRows int64
	for totalBytes > maxBytes {
		required := totalBytes - maxBytes
		ids, reclaimed, err := oldestContentRowsForTrim(gormDB, required, 200)
		if err != nil {
			return deletedRows, err
		}
		if len(ids) == 0 || reclaimed <= 0 {
			break
		}
		result := gormDB.Where("log_id IN ?", ids).Delete(&RequestLogContent{})
		if result.Error != nil {
			return deletedRows, fmt.Errorf("usage: delete oversized content rows: %w", result.Error)
		}
		deletedRows += result.RowsAffected
		totalBytes -= reclaimed
	}
	return deletedRows, nil
}

func queryStoredContentBytes(gormDB *gorm.DB) (int64, error) {
	if gormDB == nil {
		return 0, nil
	}
	var totalBytes int64
	err := gormDB.Model(&RequestLogContent{}).
		Select("COALESCE(SUM(CAST(length(input_content) AS INTEGER) + CAST(length(output_content) AS INTEGER) + CAST(length(detail_content) AS INTEGER)), 0)").
		Scan(&totalBytes).Error
	if err != nil {
		return 0, fmt.Errorf("usage: query stored content bytes: %w", err)
	}
	return totalBytes, nil
}

func oldestContentRowsForTrim(gormDB *gorm.DB, requiredBytes int64, limit int) ([]int64, int64, error) {
	if gormDB == nil || requiredBytes <= 0 {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 200
	}

	type rowResult struct {
		LogID int64
		Size  int64
	}
	var rows []rowResult
	err := gormDB.Model(&RequestLogContent{}).
		Select("log_id, CAST(length(input_content) AS INTEGER) + CAST(length(output_content) AS INTEGER) + CAST(length(detail_content) AS INTEGER) AS size").
		Order("timestamp ASC, log_id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("usage: query oldest content rows: %w", err)
	}

	ids := make([]int64, 0, len(rows))
	var reclaimed int64
	for _, row := range rows {
		ids = append(ids, row.LogID)
		reclaimed += row.Size
		if reclaimed >= requiredBytes {
			break
		}
	}
	return ids, reclaimed, nil
}

// runSQLiteCompaction runs SQLite-specific compaction (checkpoint + conditional vacuum).
// It obtains the underlying *sql.DB from gormDB for PRAGMA/VACUUM operations.
func runSQLiteCompaction(gormDB *gorm.DB, allowOptimize bool) {
	if gormDB == nil {
		return
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return
	}
	compactLogContentStorageInternal(sqlDB, allowOptimize)
}

func compactLogContentStorage() {
	if !db.IsSQLite() {
		return
	}
	gormDB := getGormDB()
	if gormDB == nil {
		return
	}
	runSQLiteCompaction(gormDB, true)
}

type sqliteSpaceStats struct {
	PageSize      int64
	PageCount     int64
	FreeListCount int64
}

func querySQLiteSpaceStats(sqlDB *sql.DB) (sqliteSpaceStats, error) {
	if sqlDB == nil {
		return sqliteSpaceStats{}, fmt.Errorf("usage: nil db")
	}
	var pageSize int64
	if err := sqlDB.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return sqliteSpaceStats{}, err
	}
	var pageCount int64
	if err := sqlDB.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return sqliteSpaceStats{}, err
	}
	var freeListCount int64
	if err := sqlDB.QueryRow("PRAGMA freelist_count").Scan(&freeListCount); err != nil {
		return sqliteSpaceStats{}, err
	}
	return sqliteSpaceStats{
		PageSize:      pageSize,
		PageCount:     pageCount,
		FreeListCount: freeListCount,
	}, nil
}

func reclaimableBytes(stats sqliteSpaceStats) int64 {
	if stats.PageSize <= 0 || stats.FreeListCount <= 0 {
		return 0
	}
	return stats.PageSize * stats.FreeListCount
}

func shouldVacuum(stats sqliteSpaceStats) bool {
	if stats.PageSize <= 0 || stats.PageCount <= 0 || stats.FreeListCount <= 0 {
		return false
	}

	freeBytes := reclaimableBytes(stats)
	if freeBytes < sqliteVacuumMinReclaimBytes {
		ratio := float64(stats.FreeListCount) / float64(stats.PageCount)
		return ratio >= sqliteVacuumMinReclaimRatio && freeBytes >= (sqliteVacuumMinReclaimBytes/2)
	}
	return true
}

func vacuumAllowedNow(now time.Time) bool {
	lastNano := lastUsageVacuumUnixNano.Load()
	if lastNano <= 0 {
		return true
	}
	last := time.Unix(0, lastNano)
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= sqliteVacuumMinInterval
}

func markVacuumRan(now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	lastUsageVacuumUnixNano.Store(now.UnixNano())
}

func usageWALPath() string {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()
	if usageDBPath == "" {
		return ""
	}
	return usageDBPath + "-wal"
}

func walBytesOnDisk() int64 {
	path := usageWALPath()
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func compactLogContentStorageInternal(sqlDB *sql.DB, allowOptimize bool) {
	if sqlDB == nil {
		return
	}

	if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Warnf("usage: wal checkpoint failed: %v", err)
	}

	stats, errStats := querySQLiteSpaceStats(sqlDB)
	if errStats != nil {
		if allowOptimize {
			if _, err := sqlDB.Exec("PRAGMA optimize"); err != nil {
				log.Warnf("usage: sqlite optimize failed: %v", err)
			}
		}
		return
	}

	didVacuum := false
	now := time.Now()
	if requestLogStorage.VacuumOnCleanup && shouldVacuum(stats) && vacuumAllowedNow(now) {
		freeBytes := reclaimableBytes(stats)
		log.Infof("usage: reclaimable sqlite free space detected (freelist=%d pages, approx=%d bytes), running VACUUM", stats.FreeListCount, freeBytes)
		if _, err := sqlDB.Exec("VACUUM"); err != nil {
			log.Warnf("usage: vacuum failed: %v", err)
		} else {
			didVacuum = true
			markVacuumRan(now)
		}
	}

	// Optimize when asked (maintenance pass) or after a successful VACUUM.
	if allowOptimize || didVacuum {
		if _, err := sqlDB.Exec("PRAGMA optimize"); err != nil {
			log.Warnf("usage: sqlite optimize failed: %v", err)
		}
	}

	// If WAL is still large after checkpoint, surface it as a hint in logs.
	if walBytes := walBytesOnDisk(); walBytes > 0 && walBytes >= (64<<20) {
		log.Warnf("usage: sqlite WAL remains large after checkpoint (%d bytes at %s); consider lowering cleanup-interval-minutes or checking long-lived transactions", walBytes, usageWALPath())
	}
}

// RequestLogContentBytes returns the cached total compressed content bytes.
// Returns -1 if unknown.
func RequestLogContentBytes() int64 {
	return requestLogContentBytes.Load()
}

// UpdateRequestLogContentBytesDelta adjusts the cached total by delta (positive or negative).
func UpdateRequestLogContentBytesDelta(delta int64) {
	requestLogContentBytes.Add(delta)
}

// InvalidateRequestLogContentBytes forces a refresh on next read.
func InvalidateRequestLogContentBytes() {
	requestLogContentBytes.Store(-1)
}
