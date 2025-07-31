package metadata

import (
	"fmt"
	"time"

	"github.com/nullable-eth/syncarr/internal/discovery"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/plex"
)

// Synchronizer handles metadata synchronization between source and destination Plex servers
type Synchronizer struct {
	sourceClient *plex.Client
	destClient   *plex.Client
	logger       *logger.Logger
}

// NewSynchronizer creates a new metadata synchronizer
func NewSynchronizer(sourceClient, destClient *plex.Client, logger *logger.Logger) *Synchronizer {
	return &Synchronizer{
		sourceClient: sourceClient,
		destClient:   destClient,
		logger:       logger,
	}
}

// SyncMetadata synchronizes metadata for a single media item using concrete plex types
func (s *Synchronizer) SyncMetadata(sourceItem interface{}, destRatingKey string) error {
	// Extract rating key and title from concrete plex types
	sourceRatingKey := s.getItemRatingKey(sourceItem)
	itemTitle := s.getItemTitle(sourceItem)

	if sourceRatingKey == "" {
		return fmt.Errorf("source item has no rating key")
	}

	s.logger.WithFields(map[string]interface{}{
		"source_rating_key": sourceRatingKey,
		"dest_rating_key":   destRatingKey,
		"title":             itemTitle,
	}).Debug("Starting comprehensive metadata synchronization")

	var syncErrors []string

	// Sync watched state
	if err := s.syncWatchedState(sourceRatingKey, destRatingKey); err != nil {
		s.logger.WithError(err).Debug("Failed to sync watched state")
		syncErrors = append(syncErrors, fmt.Sprintf("watched state: %v", err))
	}

	// Sync metadata based on item type
	switch sourceItem := sourceItem.(type) {
	case plex.Movie:
		if err := s.syncMovieMetadata(sourceItem, destRatingKey); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("movie metadata: %v", err))
		}
	case plex.TVShow:
		if err := s.syncTVShowMetadata(sourceItem, destRatingKey); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("TV show metadata: %v", err))
		}
	default:
		s.logger.WithField("item_type", fmt.Sprintf("%T", sourceItem)).Debug("Unsupported item type for comprehensive sync")
		syncErrors = append(syncErrors, "unsupported item type")
	}

	if len(syncErrors) > 0 {
		s.logger.WithFields(map[string]interface{}{
			"source_rating_key": sourceRatingKey,
			"dest_rating_key":   destRatingKey,
			"errors":            syncErrors,
		}).Warn("Some metadata synchronization operations failed")
		return fmt.Errorf("metadata sync partially failed: %v", syncErrors)
	}

	s.logger.WithFields(map[string]interface{}{
		"source_rating_key": sourceRatingKey,
		"dest_rating_key":   destRatingKey,
	}).Debug("Comprehensive metadata synchronization completed")

	return nil
}

// SyncEnhancedMetadata synchronizes comprehensive metadata using enhanced items with library context
func (s *Synchronizer) SyncEnhancedMetadata(sourceEnhanced, destEnhanced *discovery.EnhancedMediaItem) error {
	sourceRatingKey := s.getItemRatingKey(sourceEnhanced.Item)
	destRatingKey := s.getItemRatingKey(destEnhanced.Item)
	itemTitle := s.getItemTitle(sourceEnhanced.Item)

	if sourceRatingKey == "" || destRatingKey == "" {
		return fmt.Errorf("source or destination item has no rating key")
	}

	s.logger.WithFields(map[string]interface{}{
		"source_rating_key": sourceRatingKey,
		"dest_rating_key":   destRatingKey,
		"source_library":    sourceEnhanced.LibraryID,
		"dest_library":      destEnhanced.LibraryID,
		"title":             itemTitle,
	}).Debug("Starting enhanced metadata synchronization with library context")

	var syncErrors []string

	// Sync watched state
	if err := s.syncWatchedState(sourceRatingKey, destRatingKey); err != nil {
		s.logger.WithError(err).Debug("Failed to sync watched state")
		syncErrors = append(syncErrors, fmt.Sprintf("watched state: %v", err))
	}

	// Sync metadata based on item type with library context
	switch sourceItem := sourceEnhanced.Item.(type) {
	case plex.Movie:
		if err := s.syncEnhancedMovieMetadata(sourceItem, destRatingKey, destEnhanced.LibraryID); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("enhanced movie metadata: %v", err))
		}
	case plex.TVShow:
		if err := s.syncEnhancedTVShowMetadata(sourceItem, destRatingKey, destEnhanced.LibraryID); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("enhanced TV show metadata: %v", err))
		}
	default:
		s.logger.WithField("item_type", fmt.Sprintf("%T", sourceEnhanced.Item)).Debug("Unsupported item type for enhanced sync")
		syncErrors = append(syncErrors, "unsupported item type")
	}

	if len(syncErrors) > 0 {
		s.logger.WithFields(map[string]interface{}{
			"source_rating_key": sourceRatingKey,
			"dest_rating_key":   destRatingKey,
			"errors":            syncErrors,
		}).Warn("Some enhanced metadata synchronization operations failed")
		return fmt.Errorf("enhanced metadata sync partially failed: %v", syncErrors)
	}

	s.logger.WithFields(map[string]interface{}{
		"source_rating_key": sourceRatingKey,
		"dest_rating_key":   destRatingKey,
	}).Debug("Enhanced metadata synchronization completed")

	return nil
}

// syncWatchedState synchronizes watched state between source and destination
func (s *Synchronizer) syncWatchedState(sourceRatingKey, destRatingKey string) error {
	// Get watched state from source
	sourceWatchedState, err := s.sourceClient.GetWatchedState(sourceRatingKey)
	if err != nil {
		return fmt.Errorf("failed to get source watched state: %w", err)
	}

	// Get watched state from destination
	destWatchedState, err := s.destClient.GetWatchedState(destRatingKey)
	if err != nil {
		return fmt.Errorf("failed to get destination watched state: %w", err)
	}

	// Determine which state is more recent and sync accordingly
	syncToDest := false
	syncToSource := false

	// If source is watched but destination is not, sync to destination
	if sourceWatchedState.Watched && !destWatchedState.Watched {
		if destWatchedState.LastViewedAt == 0 ||
			sourceWatchedState.LastViewedAt > destWatchedState.LastViewedAt {
			syncToDest = true
		}
	}

	// If destination is watched but source is not, sync to source
	if !sourceWatchedState.Watched && destWatchedState.Watched {
		if sourceWatchedState.LastViewedAt == 0 ||
			destWatchedState.LastViewedAt > sourceWatchedState.LastViewedAt {
			syncToSource = true
		}
	}

	// If both are watched, sync the one with the higher view count or more recent date
	if sourceWatchedState.Watched && destWatchedState.Watched {
		if sourceWatchedState.ViewCount > destWatchedState.ViewCount {
			syncToDest = true
		} else if destWatchedState.ViewCount > sourceWatchedState.ViewCount {
			syncToSource = true
		} else if sourceWatchedState.LastViewedAt > destWatchedState.LastViewedAt {
			syncToDest = true
		} else if destWatchedState.LastViewedAt > sourceWatchedState.LastViewedAt {
			syncToSource = true
		}
	}

	// Perform synchronization
	if syncToDest {
		if err := s.destClient.SetWatchedState(destRatingKey, sourceWatchedState.Watched); err != nil {
			return fmt.Errorf("failed to sync watched state to destination: %w", err)
		}
		s.logger.LogWatchedStateSync(destRatingKey, "", sourceWatchedState.Watched, destWatchedState.Watched)
	}

	if syncToSource {
		if err := s.sourceClient.SetWatchedState(sourceRatingKey, destWatchedState.Watched); err != nil {
			return fmt.Errorf("failed to sync watched state to source: %w", err)
		}
		s.logger.LogWatchedStateSync(sourceRatingKey, "", destWatchedState.Watched, sourceWatchedState.Watched)
	}

	return nil
}

// syncMovieMetadata synchronizes all movie-specific metadata fields
func (s *Synchronizer) syncMovieMetadata(sourceMovie plex.Movie, destRatingKey string) error {
	var errors []string

	// We need the library ID for some operations - for now we'll skip operations that require it
	// TODO: Pass library ID through the sync chain

	// Sync user rating
	if sourceMovie.UserRating.Value > 0 {
		if err := s.destClient.SetUserRating(destRatingKey, sourceMovie.UserRating.Value); err != nil {
			s.logger.WithError(err).Debug("Failed to sync user rating")
			errors = append(errors, fmt.Sprintf("user rating: %v", err))
		}
	}

	// Sync labels (requires library ID - skip for now)
	if len(sourceMovie.Label) > 0 {
		s.logger.Debug("Label sync requires library ID - skipping for now")
		// labels := s.extractMovieLabels(sourceMovie)
		// if err := s.destClient.SetLabels(destRatingKey, libraryID, labels); err != nil {
		//     errors = append(errors, fmt.Sprintf("labels: %v", err))
		// }
	}

	if len(errors) > 0 {
		return fmt.Errorf("movie metadata sync errors: %v", errors)
	}

	s.logger.WithField("dest_rating_key", destRatingKey).Debug("Movie metadata sync completed")
	return nil
}

// syncTVShowMetadata synchronizes all TV show-specific metadata fields
func (s *Synchronizer) syncTVShowMetadata(sourceTVShow plex.TVShow, destRatingKey string) error {
	var errors []string

	// Sync user rating
	if sourceTVShow.UserRating.Value > 0 {
		if err := s.destClient.SetUserRating(destRatingKey, sourceTVShow.UserRating.Value); err != nil {
			s.logger.WithError(err).Debug("Failed to sync user rating")
			errors = append(errors, fmt.Sprintf("user rating: %v", err))
		}
	}

	// Sync labels (requires library ID - skip for now)
	if len(sourceTVShow.Label) > 0 {
		s.logger.Debug("Label sync requires library ID - skipping for now")
		// labels := s.extractTVShowLabels(sourceTVShow)
		// if err := s.destClient.SetLabels(destRatingKey, libraryID, labels); err != nil {
		//     errors = append(errors, fmt.Sprintf("labels: %v", err))
		// }
	}

	if len(errors) > 0 {
		return fmt.Errorf("TV show metadata sync errors: %v", errors)
	}

	s.logger.WithField("dest_rating_key", destRatingKey).Debug("TV show metadata sync completed")
	return nil
}

// syncEnhancedMovieMetadata synchronizes all movie metadata fields with library context
func (s *Synchronizer) syncEnhancedMovieMetadata(sourceMovie plex.Movie, destRatingKey, destLibraryID string) error {
	var errors []string

	// Sync user rating
	if sourceMovie.UserRating.Value > 0 {
		if err := s.destClient.SetUserRating(destRatingKey, sourceMovie.UserRating.Value); err != nil {
			s.logger.WithError(err).Debug("Failed to sync user rating")
			errors = append(errors, fmt.Sprintf("user rating: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key": destRatingKey,
				"rating":     sourceMovie.UserRating.Value,
			}).Debug("Synced user rating")
		}
	}

	// Sync labels - now we have the library ID!
	if len(sourceMovie.Label) > 0 {
		labels := s.extractMovieLabels(sourceMovie)
		if err := s.destClient.SetLabels(destRatingKey, destLibraryID, labels); err != nil {
			s.logger.WithError(err).Debug("Failed to sync labels")
			errors = append(errors, fmt.Sprintf("labels: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key":  destRatingKey,
				"library_id":  destLibraryID,
				"labels":      labels,
				"label_count": len(labels),
			}).Debug("Synced labels")
		}
	}

	// Sync genres using the existing UpdateMediaField method
	if len(sourceMovie.Genre) > 0 {
		genres := s.extractMovieGenres(sourceMovie)
		if err := s.destClient.UpdateMediaField(destRatingKey, destLibraryID, genres, "genre", "movie"); err != nil {
			s.logger.WithError(err).Debug("Failed to sync genres")
			errors = append(errors, fmt.Sprintf("genres: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key":  destRatingKey,
				"library_id":  destLibraryID,
				"genres":      genres,
				"genre_count": len(genres),
			}).Debug("Synced genres")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("enhanced movie metadata sync errors: %v", errors)
	}

	s.logger.WithFields(map[string]interface{}{
		"dest_rating_key": destRatingKey,
		"dest_library_id": destLibraryID,
	}).Debug("Enhanced movie metadata sync completed")
	return nil
}

// syncEnhancedTVShowMetadata synchronizes all TV show metadata fields with library context
func (s *Synchronizer) syncEnhancedTVShowMetadata(sourceTVShow plex.TVShow, destRatingKey, destLibraryID string) error {
	var errors []string

	// Sync user rating
	if sourceTVShow.UserRating.Value > 0 {
		if err := s.destClient.SetUserRating(destRatingKey, sourceTVShow.UserRating.Value); err != nil {
			s.logger.WithError(err).Debug("Failed to sync user rating")
			errors = append(errors, fmt.Sprintf("user rating: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key": destRatingKey,
				"rating":     sourceTVShow.UserRating.Value,
			}).Debug("Synced user rating")
		}
	}

	// Sync labels - now we have the library ID!
	if len(sourceTVShow.Label) > 0 {
		labels := s.extractTVShowLabels(sourceTVShow)
		if err := s.destClient.SetLabels(destRatingKey, destLibraryID, labels); err != nil {
			s.logger.WithError(err).Debug("Failed to sync labels")
			errors = append(errors, fmt.Sprintf("labels: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key":  destRatingKey,
				"library_id":  destLibraryID,
				"labels":      labels,
				"label_count": len(labels),
			}).Debug("Synced labels")
		}
	}

	// Sync genres using the existing UpdateMediaField method
	if len(sourceTVShow.Genre) > 0 {
		genres := s.extractTVShowGenres(sourceTVShow)
		if err := s.destClient.UpdateMediaField(destRatingKey, destLibraryID, genres, "genre", "show"); err != nil {
			s.logger.WithError(err).Debug("Failed to sync genres")
			errors = append(errors, fmt.Sprintf("genres: %v", err))
		} else {
			s.logger.WithFields(map[string]interface{}{
				"rating_key":  destRatingKey,
				"library_id":  destLibraryID,
				"genres":      genres,
				"genre_count": len(genres),
			}).Debug("Synced genres")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("enhanced TV show metadata sync errors: %v", errors)
	}

	s.logger.WithFields(map[string]interface{}{
		"dest_rating_key": destRatingKey,
		"dest_library_id": destLibraryID,
	}).Debug("Enhanced TV show metadata sync completed")
	return nil
}

// extractMovieLabels extracts label strings from a Movie
func (s *Synchronizer) extractMovieLabels(movie plex.Movie) []string {
	var labels []string
	for _, label := range movie.Label {
		labels = append(labels, label.Tag)
	}
	return labels
}

// extractTVShowLabels extracts label strings from a TV Show
func (s *Synchronizer) extractTVShowLabels(tvshow plex.TVShow) []string {
	var labels []string
	for _, label := range tvshow.Label {
		labels = append(labels, label.Tag)
	}
	return labels
}

// extractMovieGenres extracts genre strings from a Movie
func (s *Synchronizer) extractMovieGenres(movie plex.Movie) []string {
	var genres []string
	for _, genre := range movie.Genre {
		genres = append(genres, genre.Tag)
	}
	return genres
}

// extractTVShowGenres extracts genre strings from a TV Show
func (s *Synchronizer) extractTVShowGenres(tvshow plex.TVShow) []string {
	var genres []string
	for _, genre := range tvshow.Genre {
		genres = append(genres, genre.Tag)
	}
	return genres
}

// SyncBulkMetadata synchronizes metadata for multiple items using concrete plex types
func (s *Synchronizer) SyncBulkMetadata(items []MetadataSync) error {
	for i, item := range items {
		itemTitle := s.getItemTitle(item.SourceItem)
		sourceRatingKey := s.getItemRatingKey(item.SourceItem)

		s.logger.WithFields(map[string]interface{}{
			"progress": fmt.Sprintf("%d/%d", i+1, len(items)),
			"title":    itemTitle,
		}).Debug("Processing metadata sync")

		if err := s.SyncMetadata(item.SourceItem, item.DestRatingKey); err != nil {
			s.logger.LogError(err, map[string]interface{}{
				"source_rating_key": sourceRatingKey,
				"dest_rating_key":   item.DestRatingKey,
				"title":             itemTitle,
			})
			// Continue with other items even if one fails
		}

		// Small delay to avoid overwhelming the servers
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// MetadataSync represents a metadata synchronization operation using concrete plex types
type MetadataSync struct {
	SourceItem    interface{} `json:"sourceItem"` // Concrete plex types (Movie, TVShow, Episode)
	DestRatingKey string      `json:"destRatingKey"`
}

// ValidateMetadataConsistency checks if metadata is consistent between source and destination
func (s *Synchronizer) ValidateMetadataConsistency(sourceRatingKey, destRatingKey string) (*ConsistencyReport, error) {
	report := &ConsistencyReport{
		SourceRatingKey: sourceRatingKey,
		DestRatingKey:   destRatingKey,
		Issues:          []string{},
		Timestamp:       time.Now(),
	}

	// Check watched state consistency
	sourceWatched, err := s.sourceClient.GetWatchedState(sourceRatingKey)
	if err != nil {
		report.Issues = append(report.Issues, fmt.Sprintf("Failed to get source watched state: %v", err))
		return report, nil
	}

	destWatched, err := s.destClient.GetWatchedState(destRatingKey)
	if err != nil {
		report.Issues = append(report.Issues, fmt.Sprintf("Failed to get destination watched state: %v", err))
		return report, nil
	}

	if sourceWatched.Watched != destWatched.Watched {
		report.Issues = append(report.Issues,
			fmt.Sprintf("Watched state mismatch: source=%t, dest=%t",
				sourceWatched.Watched, destWatched.Watched))
	}

	if abs(sourceWatched.ViewCount-destWatched.ViewCount) > 1 {
		report.Issues = append(report.Issues,
			fmt.Sprintf("View count mismatch: source=%d, dest=%d",
				sourceWatched.ViewCount, destWatched.ViewCount))
	}

	report.IsConsistent = len(report.Issues) == 0
	return report, nil
}

// ConsistencyReport represents the result of a metadata consistency check
type ConsistencyReport struct {
	SourceRatingKey string    `json:"sourceRatingKey"`
	DestRatingKey   string    `json:"destRatingKey"`
	IsConsistent    bool      `json:"isConsistent"`
	Issues          []string  `json:"issues"`
	Timestamp       time.Time `json:"timestamp"`
}

// Helper functions

// getItemRatingKey safely extracts rating key from concrete plex types
func (s *Synchronizer) getItemRatingKey(item interface{}) string {
	switch v := item.(type) {
	case plex.Movie:
		return v.RatingKey.String()
	case plex.TVShow:
		return v.RatingKey.String()
	case plex.Episode:
		return v.RatingKey.String()
	default:
		return ""
	}
}

// getItemTitle safely extracts title from concrete plex types
func (s *Synchronizer) getItemTitle(item interface{}) string {
	switch v := item.(type) {
	case plex.Movie:
		return v.Title
	case plex.TVShow:
		return v.Title
	case plex.Episode:
		return v.Title
	default:
		return "unknown"
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
