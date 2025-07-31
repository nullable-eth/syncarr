package discovery

import (
	"fmt"
	"path/filepath"

	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/plex"
)

// ContentMatcher handles Phase 5: Content Matching
type ContentMatcher struct {
	sourceClient *plex.Client
	destClient   *plex.Client
	logger       *logger.Logger
}

// ItemMatch represents a matched item between source and destination with full metadata
type ItemMatch struct {
	SourceItem *EnhancedMediaItem
	DestItem   *EnhancedMediaItem
	Filename   string
}

// NewContentMatcher creates a new content matcher
func NewContentMatcher(sourceClient, destClient *plex.Client, log *logger.Logger) *ContentMatcher {
	return &ContentMatcher{
		sourceClient: sourceClient,
		destClient:   destClient,
		logger:       log,
	}
}

// MatchItemsByFilename implements Phase 5: Content Matching by filename with full metadata
func (cm *ContentMatcher) MatchItemsByFilename(sourceItems []*EnhancedMediaItem) ([]ItemMatch, error) {
	cm.logger.Info("Phase 5: Starting enhanced content matching by filename with full metadata loading")

	// Get all items from destination server and load their full metadata
	destLibraries, err := cm.destClient.GetLibraries()
	if err != nil {
		return nil, fmt.Errorf("failed to get destination libraries: %w", err)
	}

	var allDestItems []*EnhancedMediaItem
	for _, library := range destLibraries {
		cm.logger.WithFields(map[string]interface{}{
			"library_id":    library.Key,
			"library_title": library.Title,
		}).Debug("Retrieving items from destination library with full metadata")

		// Get library content using Key
		items, err := cm.destClient.GetLibraryContent(library.Key)
		if err != nil {
			cm.logger.WithError(err).WithField("library_id", library.Key).Warn("Failed to get content from destination library")
			continue
		}

		// Load full metadata for each destination item
		for i, item := range items {
			cm.logger.WithFields(map[string]interface{}{
				"progress": fmt.Sprintf("%d/%d", i+1, len(items)),
				"library":  library.Title,
			}).Debug("Loading full metadata for destination item")

			enhancedItem, err := cm.loadDestinationFullMetadata(item, library.Key, library.Type)
			if err != nil {
				cm.logger.WithError(err).WithField("item", fmt.Sprintf("%T", item)).Debug("Failed to load full metadata for destination item")
				continue
			}

			if enhancedItem != nil {
				allDestItems = append(allDestItems, enhancedItem)
			}
		}
	}

	// Build filename index for destination items with enhanced metadata
	destFileIndex := make(map[string]*EnhancedMediaItem)
	for _, enhancedItem := range allDestItems {
		// Extract file paths from the enhanced item
		filePaths := cm.extractEnhancedFilePaths(enhancedItem)
		for _, filePath := range filePaths {
			filename := filepath.Base(filePath)
			if filename != "" {
				destFileIndex[filename] = enhancedItem
			}
		}
	}

	cm.logger.WithFields(map[string]interface{}{
		"dest_items":    len(allDestItems),
		"indexed_files": len(destFileIndex),
	}).Info("Built enhanced destination file index with full metadata")

	// Match source items to destination items
	var matches []ItemMatch
	for _, sourceEnhanced := range sourceItems {
		// Extract file paths from source enhanced item
		sourceFilePaths := cm.extractEnhancedFilePaths(sourceEnhanced)

		for _, sourceFilePath := range sourceFilePaths {
			sourceFilename := filepath.Base(sourceFilePath)
			if sourceFilename == "" {
				continue
			}

			// Look for exact filename match
			if destEnhanced, exists := destFileIndex[sourceFilename]; exists {
				match := ItemMatch{
					SourceItem: sourceEnhanced,
					DestItem:   destEnhanced,
					Filename:   sourceFilename,
				}
				matches = append(matches, match)

				cm.logger.WithFields(map[string]interface{}{
					"filename":    sourceFilename,
					"source_item": cm.getEnhancedItemTitle(sourceEnhanced),
					"dest_item":   cm.getEnhancedItemTitle(destEnhanced),
				}).Debug("Found enhanced filename match with full metadata")

				break // Only match once per source item
			}
		}
	}

	cm.logger.WithFields(map[string]interface{}{
		"source_items": len(sourceItems),
		"matches":      len(matches),
	}).Info("Enhanced content matching with full metadata complete")

	return matches, nil
}

// extractFilePaths extracts file paths from metadata
func (cm *ContentMatcher) extractFilePaths(item interface{}) []string {
	var paths []string

	switch v := item.(type) {
	case plex.Movie:
		for _, media := range v.Media {
			for _, part := range media.Part {
				if part.File != "" {
					paths = append(paths, part.File)
				}
			}
		}
	case plex.TVShow:
		for _, media := range v.Media {
			for _, part := range media.Part {
				if part.File != "" {
					paths = append(paths, part.File)
				}
			}
		}
	case plex.Episode:
		for _, media := range v.Media {
			for _, part := range media.Part {
				if part.File != "" {
					paths = append(paths, part.File)
				}
			}
		}
	}

	return paths
}

// getItemTitle safely extracts title from an item
func (cm *ContentMatcher) getItemTitle(item interface{}) string {
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

// loadDestinationFullMetadata loads complete metadata for a destination item
func (cm *ContentMatcher) loadDestinationFullMetadata(item interface{}, libraryID, libraryType string) (*EnhancedMediaItem, error) {
	// Get the rating key from the basic item
	ratingKey := cm.getRatingKey(item)
	if ratingKey == "" {
		return nil, fmt.Errorf("item has no rating key")
	}

	// Load full metadata based on item type
	switch item.(type) {
	case plex.Movie:
		fullMovie, err := cm.destClient.GetMovieDetails(ratingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load full destination movie metadata: %w", err)
		}
		return &EnhancedMediaItem{
			Item:      *fullMovie,
			LibraryID: libraryID,
			ItemType:  "movie",
		}, nil

	case plex.TVShow:
		fullTVShow, err := cm.destClient.GetTVShowDetails(ratingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load full destination TV show metadata: %w", err)
		}
		return &EnhancedMediaItem{
			Item:      *fullTVShow,
			LibraryID: libraryID,
			ItemType:  "show",
		}, nil

	case plex.Episode:
		// For episodes, use the basic item for now
		return &EnhancedMediaItem{
			Item:      item,
			LibraryID: libraryID,
			ItemType:  "episode",
		}, nil

	default:
		return nil, fmt.Errorf("unsupported destination item type: %T", item)
	}
}

// getRatingKey safely extracts rating key from any item type
func (cm *ContentMatcher) getRatingKey(item interface{}) string {
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

// extractEnhancedFilePaths extracts file paths from an enhanced media item
func (cm *ContentMatcher) extractEnhancedFilePaths(enhancedItem *EnhancedMediaItem) []string {
	return cm.extractFilePaths(enhancedItem.Item)
}

// getEnhancedItemTitle safely extracts title from an enhanced media item
func (cm *ContentMatcher) getEnhancedItemTitle(enhancedItem *EnhancedMediaItem) string {
	return cm.getItemTitle(enhancedItem.Item)
}
