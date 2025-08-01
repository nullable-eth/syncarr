package discovery

import (
	"fmt"

	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/plex"
)

// EnhancedMediaItem wraps Plex media items with library context and full metadata
type EnhancedMediaItem struct {
	Item      interface{} // plex.Movie, plex.TVShow, or plex.Episode with FULL metadata
	LibraryID string      // Library ID for API operations
	ItemType  string      // "movie", "show", "episode"
}

// ContentDiscovery implements Phase 1: Complete Library Scanning
type ContentDiscovery struct {
	sourceClient *plex.Client
	syncLabel    string
	logger       *logger.Logger
}

// NewContentDiscovery creates a new content discovery instance
func NewContentDiscovery(sourceClient *plex.Client, syncLabel string, logger *logger.Logger) *ContentDiscovery {
	return &ContentDiscovery{
		sourceClient: sourceClient,
		syncLabel:    syncLabel,
		logger:       logger,
	}
}

// DiscoverSyncableContent implements Phase 1 and 2 from the implementation plan:
//  1. List all items from all libraries on the source server with FULL metadata
//  2. If any movie contains the sync tag, add it to the processing list with complete metadata
//     If any TV show contains the sync label, list all episodes of all seasons and add them with complete metadata
func (cd *ContentDiscovery) DiscoverSyncableContent() ([]*EnhancedMediaItem, error) {
	cd.logger.Debug("Phase 1: Starting enhanced content discovery with full metadata loading")

	var itemsToSync []*EnhancedMediaItem

	// Get all libraries from source server
	libraries, err := cd.sourceClient.GetLibraries()
	if err != nil {
		return nil, fmt.Errorf("failed to get libraries: %w", err)
	}

	cd.logger.WithField("library_count", len(libraries)).Debug("Retrieved libraries from source server")

	for _, library := range libraries {
		cd.logger.WithFields(map[string]interface{}{
			"library_id":    library.Key,
			"library_title": library.Title,
		}).Debug("Scanning library for content with full metadata")

		// Get all items from this library with basic info first
		labeledItems, err := cd.sourceClient.GetItemsWithLabel(library.Key, cd.syncLabel)
		if err != nil {
			cd.logger.WithError(err).WithFields(map[string]interface{}{
				"library_id": library.Key,
				"sync_label": cd.syncLabel,
			}).Warn("Failed to get items with label")
			continue
		}

		cd.logger.WithFields(map[string]interface{}{
			"library_id":    library.Key,
			"sync_label":    cd.syncLabel,
			"labeled_items": len(labeledItems),
		}).Debug("Retrieved items with sync label, now loading full metadata")

		for i, item := range labeledItems {
			cd.logger.WithFields(map[string]interface{}{
				"progress": fmt.Sprintf("%d/%d", i+1, len(labeledItems)),
				"library":  library.Title,
			}).Debug("Loading full metadata for item")

			enhancedItem, err := cd.loadFullMetadata(item, library.Key, library.Type)
			if err != nil {
				cd.logger.WithError(err).WithField("item", fmt.Sprintf("%T", item)).Warn("Failed to load full metadata for item")
				continue
			}

			if enhancedItem != nil {
				itemsToSync = append(itemsToSync, enhancedItem)
				cd.logger.WithFields(map[string]interface{}{
					"title":      cd.getItemTitle(enhancedItem.Item),
					"item_type":  enhancedItem.ItemType,
					"library_id": enhancedItem.LibraryID,
				}).Debug("Added item with full metadata to sync list")
			}
		}
	}

	cd.logger.WithField("total_items_to_sync", len(itemsToSync)).Debug("Phase 1 and 2: Enhanced content discovery with full metadata complete")

	return itemsToSync, nil
}

// GetItemFilePaths extracts file paths from a media item
func (cd *ContentDiscovery) GetItemFilePaths(item interface{}) ([]string, error) {
	var filePaths []string

	switch v := item.(type) {
	case plex.Movie:
		for _, media := range v.Media {
			for _, part := range media.Part {
				if part.File != "" {
					filePaths = append(filePaths, part.File)
				}
			}
		}
	case plex.TVShow:
		for _, media := range v.Media {
			for _, part := range media.Part {
				if part.File != "" {
					filePaths = append(filePaths, part.File)
				}
			}
		}
	}

	return filePaths, nil
}

// loadFullMetadata loads complete metadata for an item including all labels, genres, etc.
func (cd *ContentDiscovery) loadFullMetadata(item interface{}, libraryID, libraryType string) (*EnhancedMediaItem, error) {
	// Get the rating key from the basic item
	ratingKey := cd.getRatingKey(item)
	if ratingKey == "" {
		return nil, fmt.Errorf("item has no rating key")
	}

	// Load full metadata based on item type
	switch item.(type) {
	case plex.Movie:
		fullMovie, err := cd.sourceClient.GetMovieDetails(ratingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load full movie metadata: %w", err)
		}
		return &EnhancedMediaItem{
			Item:      *fullMovie,
			LibraryID: libraryID,
			ItemType:  "movie",
		}, nil

	case plex.TVShow:
		fullTVShow, err := cd.sourceClient.GetTVShowDetails(ratingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load full TV show metadata: %w", err)
		}
		return &EnhancedMediaItem{
			Item:      *fullTVShow,
			LibraryID: libraryID,
			ItemType:  "show",
		}, nil

	case plex.Episode:
		// For episodes, we could add a GetEpisodeDetails method if needed
		// For now, episodes from GetItemsWithLabel should have sufficient metadata
		return &EnhancedMediaItem{
			Item:      item,
			LibraryID: libraryID,
			ItemType:  "episode",
		}, nil

	default:
		return nil, fmt.Errorf("unsupported item type: %T", item)
	}
}

// getRatingKey safely extracts rating key from any item type
func (cd *ContentDiscovery) getRatingKey(item interface{}) string {
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

// getItemTitle safely extracts title from any item type
func (cd *ContentDiscovery) getItemTitle(item interface{}) string {
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

// GetEnhancedItemFilePaths extracts file paths from an enhanced media item
func (cd *ContentDiscovery) GetEnhancedItemFilePaths(enhancedItem *EnhancedMediaItem) ([]string, error) {
	return cd.GetItemFilePaths(enhancedItem.Item)
}
