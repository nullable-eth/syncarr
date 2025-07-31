package types

import (
	"time"
)

// SyncableItem represents an item that can be synchronized (kept for backward compatibility)
type SyncableItem struct {
	RatingKey    string                 `json:"ratingKey"`
	Title        string                 `json:"title"`
	LibraryID    string                 `json:"libraryId"`
	FilePaths    []string               `json:"filePaths"`
	CustomFields map[string]interface{} `json:"customFields"`
	Metadata     interface{}            `json:"metadata"` // Can hold Movie, TVShow, or Episode
}

// WatchedState represents the watch status of a media item
type WatchedState struct {
	Watched      bool      `json:"watched"`
	ViewCount    int       `json:"viewCount"`
	LastViewedAt time.Time `json:"lastViewedAt"`
	ViewOffset   int64     `json:"viewOffset"` // Resume position in milliseconds
}

// Library represents a Plex library (kept for backward compatibility)
type Library struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Path  string `json:"path"`
}

// FileTransfer represents a file transfer operation
type FileTransfer struct {
	SourcePath string `json:"sourcePath"`
	DestPath   string `json:"destPath"`
	Size       int64  `json:"size"`
}

// SyncError represents a synchronization error
type SyncError struct {
	Type        string                 `json:"type"`
	Message     string                 `json:"message"`
	Item        string                 `json:"item"`
	LibraryID   string                 `json:"libraryId"`
	Timestamp   time.Time              `json:"timestamp"`
	Details     map[string]interface{} `json:"details"`
	Recoverable bool                   `json:"recoverable"`
}

// SyncStats represents synchronization statistics
type SyncStats struct {
	StartTime            time.Time `json:"startTime"`
	EndTime              time.Time `json:"endTime"`
	Duration             time.Duration
	ItemsProcessed       int   `json:"itemsProcessed"`
	ItemsSkipped         int   `json:"itemsSkipped"`
	ItemsFailures        int   `json:"itemsFailures"`
	Errors               int   `json:"errors"` // Alias for ItemsFailures for backward compatibility
	FilesTransferred     int   `json:"filesTransferred"`
	BytesTransferred     int64 `json:"bytesTransferred"`
	WatchedStatesSynced  int   `json:"watchedStatesSynced"`
	MetadataFieldsSynced int   `json:"metadataFieldsSynced"`
}

// FailedItem represents an item that failed processing
type FailedItem struct {
	ID            string       `json:"id"` // RatingKey for backward compatibility
	Item          SyncableItem `json:"item"`
	Error         string       `json:"error"`
	Timestamp     time.Time    `json:"timestamp"`
	RetryCount    int          `json:"retryCount"`
	MaxRetries    int          `json:"maxRetries"`
	NextRetryTime time.Time    `json:"nextRetryTime"`
	Permanent     bool         `json:"permanent"`
}

// NewSyncableItem creates a SyncableItem from labelarr plex types (convenience function)
func NewSyncableItem(ratingKey, title, libraryID string, filePaths []string) SyncableItem {
	return SyncableItem{
		RatingKey:    ratingKey,
		Title:        title,
		LibraryID:    libraryID,
		FilePaths:    filePaths,
		CustomFields: make(map[string]interface{}),
		Metadata:     nil,
	}
}

// NewSyncableItemWithMetadata creates a SyncableItem with metadata from labelarr plex types
func NewSyncableItemWithMetadata(ratingKey, title, libraryID string, filePaths []string, metadata interface{}) SyncableItem {
	return SyncableItem{
		RatingKey:    ratingKey,
		Title:        title,
		LibraryID:    libraryID,
		FilePaths:    filePaths,
		CustomFields: make(map[string]interface{}),
		Metadata:     metadata,
	}
}

// NewFailedItem creates a FailedItem with proper ID field
func NewFailedItem(item SyncableItem, error string) FailedItem {
	return FailedItem{
		ID:            item.RatingKey, // Use RatingKey as ID
		Item:          item,
		Error:         error,
		Timestamp:     time.Now(),
		RetryCount:    0,
		MaxRetries:    3,
		NextRetryTime: time.Now().Add(time.Minute * 5),
		Permanent:     false,
	}
}
