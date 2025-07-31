package logger

import (
	"os"
	"time"

	"github.com/nullable-eth/syncarr/pkg/types"
	"github.com/sirupsen/logrus"
)

// Logger wraps logrus with our custom functionality
type Logger struct {
	*logrus.Logger
}

// New creates a new logger with the specified log level
func New(level string) *Logger {
	logger := logrus.New()

	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Set formatter
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	// Set output
	logger.SetOutput(os.Stdout)

	return &Logger{Logger: logger}
}

// LogSyncStart logs the beginning of a sync cycle
func (l *Logger) LogSyncStart(itemCount int) {
	l.WithFields(logrus.Fields{
		"event":      "sync_start",
		"item_count": itemCount,
	}).Info("Starting sync cycle")
}

// LogItemProcessed logs successful processing of a media item
func (l *Logger) LogItemProcessed(item types.SyncableItem, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"event":       "item_processed",
		"rating_key":  item.RatingKey,
		"title":       item.Title,
		"duration_ms": duration.Milliseconds(),
	}).Debug("Media item processed successfully")
}

// LogItemSkipped logs when a media item was skipped (unchanged)
func (l *Logger) LogItemSkipped(item types.SyncableItem, reason string) {
	l.WithFields(logrus.Fields{
		"event":      "item_skipped",
		"rating_key": item.RatingKey,
		"title":      item.Title,
		"reason":     reason,
	}).Debug("Media item skipped")
}

// LogTransferStarted logs when a file transfer begins
func (l *Logger) LogTransferStarted(sourcePath, destPath string, sizeBytes int64) {
	l.WithFields(logrus.Fields{
		"event":       "transfer_started",
		"source_path": sourcePath,
		"dest_path":   destPath,
		"size_bytes":  sizeBytes,
	}).Info("File transfer started")
}

// LogTransferCompleted logs when a file transfer completes
func (l *Logger) LogTransferCompleted(sourcePath, destPath string, sizeBytes int64, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"event":       "transfer_completed",
		"source_path": sourcePath,
		"dest_path":   destPath,
		"size_bytes":  sizeBytes,
		"duration_ms": duration.Milliseconds(),
		"rate_mbps":   float64(sizeBytes*8) / float64(duration.Seconds()) / 1000000, // Mbps
	}).Info("File transfer completed")
}

// LogError logs an error with context
func (l *Logger) LogError(err error, context map[string]interface{}) {
	fields := logrus.Fields{
		"event": "error",
		"error": err.Error(),
	}

	// Add context fields
	for k, v := range context {
		fields[k] = v
	}

	l.WithFields(fields).Error("An error occurred")
}

// LogSyncError logs a sync-specific error
func (l *Logger) LogSyncError(syncErr types.SyncError) {
	l.WithFields(logrus.Fields{
		"event":       "sync_error",
		"error_type":  syncErr.Type,
		"message":     syncErr.Message,
		"item":        syncErr.Item,
		"library_id":  syncErr.LibraryID,
		"recoverable": syncErr.Recoverable,
	}).Error("Sync error occurred")
}

// LogSyncComplete logs the completion of a sync cycle
func (l *Logger) LogSyncComplete(stats types.SyncStats) {
	l.WithFields(logrus.Fields{
		"event":                 "sync_complete",
		"items_processed":       stats.ItemsProcessed,
		"items_failed":          stats.ItemsFailures,
		"items_skipped":         stats.ItemsSkipped,
		"files_transferred":     stats.FilesTransferred,
		"bytes_transferred":     stats.BytesTransferred,
		"watched_states_synced": stats.WatchedStatesSynced,
		"duration_ms":           stats.Duration.Milliseconds(),
	}).Info("Sync cycle completed")
}

// LogForceFullSync logs when force full sync is enabled
func (l *Logger) LogForceFullSync() {
	l.WithFields(logrus.Fields{
		"event": "force_full_sync",
	}).Info("Force full sync enabled - clearing all sync state")
}

// LogStateCleared logs when sync state is cleared
func (l *Logger) LogStateCleared() {
	l.WithFields(logrus.Fields{
		"event": "state_cleared",
	}).Info("Sync state cleared successfully")
}

// LogLibraryScanTriggered logs when a library scan is triggered
func (l *Logger) LogLibraryScanTriggered(libraryID, libraryName string) {
	l.WithFields(logrus.Fields{
		"event":        "library_scan_triggered",
		"library_id":   libraryID,
		"library_name": libraryName,
	}).Info("Library scan triggered")
}

// LogLibraryScanCompleted logs when a library scan completes
func (l *Logger) LogLibraryScanCompleted(libraryID, libraryName string, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"event":        "library_scan_completed",
		"library_id":   libraryID,
		"library_name": libraryName,
		"duration_ms":  duration.Milliseconds(),
	}).Info("Library scan completed")
}

// LogWatchedStateSync logs watched state synchronization
func (l *Logger) LogWatchedStateSync(ratingKey, title string, sourceWatched, destWatched bool) {
	l.WithFields(logrus.Fields{
		"event":          "watched_state_sync",
		"rating_key":     ratingKey,
		"title":          title,
		"source_watched": sourceWatched,
		"dest_watched":   destWatched,
	}).Debug("Watched state synchronized")
}

// LogRetryAttempt logs a retry attempt
func (l *Logger) LogRetryAttempt(operation string, attempt int, maxAttempts int, err error) {
	l.WithFields(logrus.Fields{
		"event":        "retry_attempt",
		"operation":    operation,
		"attempt":      attempt,
		"max_attempts": maxAttempts,
		"error":        err.Error(),
	}).Warn("Retrying operation after error")
}

// LogDeadLetterQueue logs when an item is added to the dead letter queue
func (l *Logger) LogDeadLetterQueue(item types.FailedItem) {
	l.WithFields(logrus.Fields{
		"event":       "dead_letter_queue",
		"rating_key":  item.Item.RatingKey,
		"title":       item.Item.Title,
		"error":       item.Error,
		"retry_count": item.RetryCount,
		"max_retries": item.MaxRetries,
		"next_retry":  item.NextRetryTime,
		"permanent":   item.Permanent,
	}).Warn("Item added to dead letter queue")
}

// LogWorkerPoolStarted logs when the worker pool starts
func (l *Logger) LogWorkerPoolStarted(workerCount int) {
	l.WithFields(logrus.Fields{
		"event":        "worker_pool_started",
		"worker_count": workerCount,
	}).Info("Worker pool started")
}

// LogWorkerPoolStopped logs when the worker pool stops
func (l *Logger) LogWorkerPoolStopped() {
	l.WithFields(logrus.Fields{
		"event": "worker_pool_stopped",
	}).Info("Worker pool stopped")
}

// LogRateLimitHit logs when rate limiting is triggered
func (l *Logger) LogRateLimitHit(service string, waitTime time.Duration) {
	l.WithFields(logrus.Fields{
		"event":     "rate_limit_hit",
		"service":   service,
		"wait_time": waitTime.Milliseconds(),
	}).Debug("Rate limit hit, waiting")
}

// LogBandwidthThrottled logs when bandwidth throttling is applied
func (l *Logger) LogBandwidthThrottled(currentRate, limitRate float64) {
	l.WithFields(logrus.Fields{
		"event":        "bandwidth_throttled",
		"current_mbps": currentRate,
		"limit_mbps":   limitRate,
	}).Debug("Bandwidth throttling applied")
}

// LogCompressionUsed logs when compression is used for a transfer
func (l *Logger) LogCompressionUsed(filePath string, originalSize, compressedSize int64, algorithm string) {
	compressionRatio := float64(compressedSize) / float64(originalSize)
	l.WithFields(logrus.Fields{
		"event":             "compression_used",
		"file_path":         filePath,
		"original_size":     originalSize,
		"compressed_size":   compressedSize,
		"compression_ratio": compressionRatio,
		"algorithm":         algorithm,
	}).Debug("Compression applied to transfer")
}

// LogTransferResumed logs when a transfer is resumed
func (l *Logger) LogTransferResumed(filePath string, resumePosition int64, totalSize int64) {
	l.WithFields(logrus.Fields{
		"event":           "transfer_resumed",
		"file_path":       filePath,
		"resume_position": resumePosition,
		"total_size":      totalSize,
		"progress_pct":    float64(resumePosition) / float64(totalSize) * 100,
	}).Info("File transfer resumed")
}
