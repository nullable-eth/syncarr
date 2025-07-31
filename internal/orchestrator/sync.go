package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/discovery"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/metadata"
	"github.com/nullable-eth/syncarr/internal/plex"
	"github.com/nullable-eth/syncarr/internal/transfer"
)

// SyncOrchestrator coordinates the 6-phase synchronization process
type SyncOrchestrator struct {
	config           *config.Config
	logger           *logger.Logger
	sourceClient     *plex.Client
	destClient       *plex.Client
	contentDiscovery *discovery.ContentDiscovery
	fileTransfer     transfer.FileTransferrer
	libraryManager   *discovery.LibraryManager
	contentMatcher   *discovery.ContentMatcher
	metadataSync     *metadata.Synchronizer
	lastSyncTime     time.Time
	syncedFiles      map[string]bool // Track files that should exist on destination
}

// NewSyncOrchestrator creates a new sync orchestrator with all required components
func NewSyncOrchestrator(cfg *config.Config, log *logger.Logger) (*SyncOrchestrator, error) {
	orchestrator := &SyncOrchestrator{
		config:      cfg,
		logger:      log,
		syncedFiles: make(map[string]bool),
	}

	// Initialize Plex clients
	log.Info("Creating source Plex client")
	sourceClient, err := plex.NewClient(&cfg.Source, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create source Plex client: %w", err)
	}
	orchestrator.sourceClient = sourceClient

	log.Info("Creating destination Plex client")
	destClient, err := plex.NewClient(&cfg.Destination, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination Plex client: %w", err)
	}
	orchestrator.destClient = destClient

	// Initialize content discovery (Phase 1 & 2)
	orchestrator.contentDiscovery = discovery.NewContentDiscovery(sourceClient, cfg.SyncLabel, log)

	// Phase 3: Transfer Files - Auto-detect optimal transfer method
	if isSSHConfigured(cfg.SSH, log) {
		// Auto-detect optimal transfer method (rsync preferred for performance)
		transferMethod := transfer.GetOptimalTransferMethod(log)
		fileTransfer, err := transfer.NewTransferrer(transferMethod, cfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create file transferrer: %w", err)
		}
		orchestrator.fileTransfer = fileTransfer
		log.WithField("transfer_method", string(transferMethod)).Info("High-performance file transfer enabled")
	} else {
		log.Info("SSH not configured - running in metadata-only sync mode")
	}

	// Initialize library manager (Phase 4)
	orchestrator.libraryManager = discovery.NewLibraryManager(destClient, log)

	// Initialize content matcher (Phase 5)
	orchestrator.contentMatcher = discovery.NewContentMatcher(sourceClient, destClient, log)

	// Initialize metadata synchronizer (Phase 6)
	orchestrator.metadataSync = metadata.NewSynchronizer(sourceClient, destClient, log)

	return orchestrator, nil
}

// Close closes all connections and resources
func (s *SyncOrchestrator) Close() error {
	var errs []error

	if s.fileTransfer != nil {
		if err := s.fileTransfer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close transfer client: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing orchestrator: %v", errs)
	}

	return nil
}

// RunSyncCycle executes the complete 6-phase synchronization workflow
func (s *SyncOrchestrator) RunSyncCycle() error {
	startTime := time.Now()
	s.logger.Info("Starting 6-phase synchronization cycle")

	defer func() {
		duration := time.Since(startTime)
		s.logger.WithField("total_duration", duration).Info("Sync cycle completed successfully")
		s.lastSyncTime = startTime
	}()

	// Pre-flight check: Test destination server availability
	s.logger.Debug("Testing destination server availability")
	if err := s.destClient.TestConnection(); err != nil {
		s.logger.WithError(err).Warn("Destination Plex server is not available, skipping sync cycle")
		return fmt.Errorf("destination server unavailable: %w", err)
	}
	s.logger.Info("Destination server is available, proceeding with sync")

	// Phase 1 & 2: Content Discovery and Filtering with Full Metadata
	s.logger.Info("Phase 1 & 2: Discovering and filtering syncable content with full metadata")
	itemsToSync, err := s.contentDiscovery.DiscoverSyncableContent()
	if err != nil {
		return fmt.Errorf("content discovery failed: %w", err)
	}
	s.logger.WithField("item_count", len(itemsToSync)).Info("Enhanced content discovery complete")

	if len(itemsToSync) == 0 {
		s.logger.Info("No items found for synchronization")
		return nil
	}

	// Phase 3: File Transfer (skip if SSH not configured)
	if s.fileTransfer != nil {
		s.logger.Info("Phase 3: Transferring files")

		// Clear the synced files map for this cycle
		s.syncedFiles = make(map[string]bool)

		totalItems := len(itemsToSync)
		var transferredCount, errorCount int

		for i, enhancedItem := range itemsToSync {
			s.logger.WithFields(map[string]interface{}{
				"progress":   fmt.Sprintf("%d/%d", i+1, totalItems),
				"title":      s.getEnhancedItemTitle(enhancedItem),
				"library_id": enhancedItem.LibraryID,
			}).Debug("Transferring enhanced item files")

			if err := s.transferEnhancedItemFiles(enhancedItem); err != nil {
				s.logger.WithError(err).WithField("item", s.getEnhancedItemTitle(enhancedItem)).Error("Failed to transfer enhanced item files")
				errorCount++
				continue
			}
			transferredCount++

			// Log progress summary every 100 items or at significant milestones
			if (i+1)%100 == 0 || (i+1) == totalItems || (i+1)%500 == 0 {
				s.logger.WithFields(map[string]interface{}{
					"completed": i + 1,
					"total":     totalItems,
					"progress":  fmt.Sprintf("%.1f%%", float64(i+1)/float64(totalItems)*100),
				}).Debug("File transfer progress")
			}
		}

		// Log final transfer summary
		s.logger.WithFields(map[string]interface{}{
			"total_items":  totalItems,
			"transferred":  transferredCount,
			"errors":       errorCount,
			"success_rate": fmt.Sprintf("%.1f%%", float64(transferredCount)/float64(totalItems)*100),
		}).Debug("File transfer phase complete")

		// Phase 3.5: Cleanup - Remove files on destination that aren't in current sync list
		s.logger.Info("Phase 3.5: Cleaning up orphaned files on destination")
		if err := s.cleanupOrphanedFiles(); err != nil {
			s.logger.WithError(err).Warn("Failed to cleanup orphaned files, continuing")
		} else {
			s.logger.Info("Cleanup phase complete")
		}

		// Phase 4: Library Refresh and Monitoring (only needed after file transfer)
		s.logger.Info("Phase 4: Refreshing destination libraries")
		if err := s.libraryManager.TriggerRefreshAndWait(); err != nil {
			return fmt.Errorf("library refresh failed: %w", err)
		}
		s.logger.Info("Library refresh complete")
	} else {
		s.logger.Info("Phase 3: Skipping file transfer (SSH not configured)")
		s.logger.Info("Phase 4: Skipping library refresh (no files transferred)")
	}

	// Phase 5: Content Matching
	s.logger.Info("Phase 5: Matching items by filename")
	matches, err := s.contentMatcher.MatchItemsByFilename(itemsToSync)
	if err != nil {
		return fmt.Errorf("content matching failed: %w", err)
	}
	s.logger.WithFields(map[string]interface{}{
		"source_items": len(itemsToSync),
		"matches":      len(matches),
		"success_rate": fmt.Sprintf("%.1f%%", float64(len(matches))/float64(len(itemsToSync))*100),
	}).Info("Content matching complete")

	// Phase 6: Metadata Synchronization
	s.logger.Info("Phase 6: Synchronizing metadata")
	if len(matches) == 0 {
		s.logger.Info("No matches found, skipping metadata synchronization")
	} else {
		success, errors, skipped := s.syncAllMetadata(matches)
		s.logger.WithFields(map[string]interface{}{
			"total":   len(matches),
			"success": success,
			"errors":  errors,
			"skipped": skipped,
		}).Info("Metadata synchronization complete")
	}

	s.logger.Info("🎉 Sync cycle completed successfully!")
	return nil
}

// transferEnhancedItemFiles handles file transfer for an enhanced item with path mapping
func (s *SyncOrchestrator) transferEnhancedItemFiles(enhancedItem *discovery.EnhancedMediaItem) error {
	// Extract file paths based on item type from the enhanced item
	var filePaths []string

	switch v := enhancedItem.Item.(type) {
	case plex.Movie:
		filePaths = s.extractMovieFilePaths(v)
	case plex.TVShow:
		// For TV shows, get all episodes and their file paths
		episodes, err := s.sourceClient.GetAllTVShowEpisodes(v.RatingKey.String())
		if err != nil {
			return fmt.Errorf("failed to get episodes for TV show %s: %w", v.Title, err)
		}
		for _, episode := range episodes {
			episodePaths := s.extractEpisodeFilePaths(episode)
			filePaths = append(filePaths, episodePaths...)
		}
	case plex.Episode:
		filePaths = s.extractEpisodeFilePaths(v)
	default:
		s.logger.WithField("item_type", fmt.Sprintf("%T", enhancedItem.Item)).Warn("Unknown enhanced item type for file transfer")
		return nil
	}

	// Transfer each file with path mapping
	for _, sourcePath := range filePaths {
		if sourcePath == "" {
			continue
		}

		// Map source Plex path to local path
		localPath, err := s.fileTransfer.MapSourcePathToLocal(sourcePath)
		if err != nil {
			s.logger.WithError(err).WithField("source_path", sourcePath).Error("Failed to map source path to local path")
			continue
		}

		// Check if local file exists
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			s.logger.WithField("local_path", localPath).Warn("Local file does not exist, skipping transfer")
			continue
		}

		// Map local path to destination path
		destPath, err := s.fileTransfer.MapLocalPathToDest(localPath)
		if err != nil {
			s.logger.WithError(err).WithField("local_path", localPath).Error("Failed to map local path to destination path")
			continue
		}

		// Track this file as synced (should exist on destination) before transfer
		s.syncedFiles[destPath] = true

		// Transfer the file
		if err := s.fileTransfer.TransferFile(localPath, destPath); err != nil {
			s.logger.WithError(err).WithFields(map[string]interface{}{
				"local_path": localPath,
				"dest_path":  destPath,
			}).Error("Failed to transfer file")
			continue
		}

		// Transfer completed successfully (detailed logging handled in transfer layer)
	}

	return nil
}

// findRelatedFiles finds all files in the same directory with the same prefix (up to first period)
func (s *SyncOrchestrator) findRelatedFiles(mainFilePath string) []string {
	var allPaths []string

	// Always include the main file
	allPaths = append(allPaths, mainFilePath)

	// Get directory and filename
	dir := filepath.Dir(mainFilePath)
	filename := filepath.Base(mainFilePath)

	// Extract prefix (up to first period)
	dotIndex := strings.Index(filename, ".")
	if dotIndex == -1 {
		// No dot found, use the entire filename as prefix
		return allPaths
	}

	prefix := filename[:dotIndex]

	// Map source path to local path for directory listing
	localDir, err := s.fileTransfer.MapSourcePathToLocal(dir)
	if err != nil {
		s.logger.WithError(err).WithField("source_dir", dir).Debug("Failed to map source directory to local path")
		return allPaths
	}

	// List all files in the directory
	entries, err := os.ReadDir(localDir)
	if err != nil {
		s.logger.WithError(err).WithField("local_dir", localDir).Debug("Failed to read directory for related files")
		return allPaths
	}

	// Find files with matching prefix
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		entryName := entry.Name()
		if strings.HasPrefix(entryName, prefix+".") && entryName != filename {
			// Construct the full source path for the related file
			relatedSourcePath := filepath.Join(dir, entryName)
			allPaths = append(allPaths, relatedSourcePath)
			s.logger.WithFields(map[string]interface{}{
				"main_file":    mainFilePath,
				"related_file": relatedSourcePath,
			}).Debug("Found related file")
		}
	}

	return allPaths
}

// extractMovieFilePaths extracts file paths from a Movie and includes related files
func (s *SyncOrchestrator) extractMovieFilePaths(movie plex.Movie) []string {
	var paths []string
	for _, media := range movie.Media {
		for _, part := range media.Part {
			if part.File != "" {
				relatedFiles := s.findRelatedFiles(part.File)
				paths = append(paths, relatedFiles...)
			}
		}
	}
	return paths
}

// extractEpisodeFilePaths extracts file paths from an Episode and includes related files
func (s *SyncOrchestrator) extractEpisodeFilePaths(episode plex.Episode) []string {
	var paths []string
	for _, media := range episode.Media {
		for _, part := range media.Part {
			if part.File != "" {
				relatedFiles := s.findRelatedFiles(part.File)
				paths = append(paths, relatedFiles...)
			}
		}
	}
	return paths
}

// cleanupOrphanedFiles removes files on the destination that aren't in the current sync list
func (s *SyncOrchestrator) cleanupOrphanedFiles() error {
	if s.config.DestRootDir == "" {
		s.logger.Debug("No destination root directory configured, skipping cleanup")
		return nil
	}

	s.logger.WithField("dest_root", s.config.DestRootDir).Info("Scanning destination directory for orphaned files")

	// Get list of all files in destination directory
	destFiles, err := s.fileTransfer.ListDirectoryContents(s.config.DestRootDir)
	if err != nil {
		return fmt.Errorf("failed to list destination directory contents: %w", err)
	}

	orphanedCount := 0
	for _, destFile := range destFiles {
		// Check if this file is in our current sync list
		if !s.syncedFiles[destFile] {
			s.logger.WithField("orphaned_file", destFile).Debug("Removing orphaned file from destination")

			if err := s.fileTransfer.DeleteFile(destFile); err != nil {
				s.logger.WithError(err).WithField("file", destFile).Warn("Failed to delete orphaned file")
				continue
			}
			orphanedCount++
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"synced_files":   len(s.syncedFiles),
		"dest_files":     len(destFiles),
		"orphaned_files": orphanedCount,
	}).Debug("Cleanup phase statistics")

	return nil
}

// syncAllMetadata implements Phase 6: Complete metadata transfer with comparison
func (s *SyncOrchestrator) syncAllMetadata(matches []discovery.ItemMatch) (int, int, int) {
	var successCount, errorCount, skippedCount int

	for i, match := range matches {
		s.logger.WithFields(map[string]interface{}{
			"progress": fmt.Sprintf("%d/%d", i+1, len(matches)),
			"filename": match.Filename,
			"source":   s.getEnhancedItemTitle(match.SourceItem),
			"dest":     s.getEnhancedItemTitle(match.DestItem),
		}).Debug("Checking enhanced item metadata")

		// Get destination rating key from enhanced item
		destRatingKey := s.getEnhancedItemRatingKey(match.DestItem)
		if destRatingKey == "" {
			s.logger.WithField("filename", match.Filename).Warn("Could not get destination rating key for enhanced metadata sync")
			errorCount++
			continue
		}

		// Compare enhanced metadata before syncing - now we have full metadata for both items
		needsSync, err := s.compareEnhancedMetadata(match.SourceItem, match.DestItem)
		if err != nil {
			s.logger.WithError(err).WithField("filename", match.Filename).Debug("Failed to compare enhanced metadata, will sync anyway")
			needsSync = true // Default to syncing if comparison fails
		}

		if !needsSync {
			s.logger.WithFields(map[string]interface{}{
				"filename":   match.Filename,
				"source_key": s.getEnhancedItemRatingKey(match.SourceItem),
				"dest_key":   destRatingKey,
			}).Debug("Enhanced metadata already synchronized, skipping")
			skippedCount++
		} else {
			// Sync metadata using the enhanced metadata synchronizer
			s.logger.WithFields(map[string]interface{}{
				"filename":   match.Filename,
				"source_key": s.getEnhancedItemRatingKey(match.SourceItem),
				"dest_key":   destRatingKey,
			}).Debug("Syncing enhanced metadata differences")

			// if err := s.syncEnhancedItemMetadata(match.SourceItem, match.DestItem); err != nil {
			// 	s.logger.WithError(err).WithField("filename", match.Filename).Error("Failed to sync enhanced metadata")
			// 	errorCount++
			// 	continue
			// }
			successCount++
		}

		// Log progress summary every 100 items or at significant milestones
		if (i+1)%100 == 0 || (i+1) == len(matches) || (i+1)%500 == 0 {
			s.logger.WithFields(map[string]interface{}{
				"completed": i + 1,
				"total":     len(matches),
				"progress":  fmt.Sprintf("%.1f%%", float64(i+1)/float64(len(matches))*100),
			}).Debug("Metadata sync progress")
		}
	}

	// Log final metadata sync summary
	s.logger.WithFields(map[string]interface{}{
		"total_matches": len(matches),
		"synced":        successCount,
		"skipped":       skippedCount,
		"errors":        errorCount,
		"sync_rate":     fmt.Sprintf("%.1f%%", float64(successCount)/float64(len(matches))*100),
	}).Debug("Metadata synchronization complete")

	return successCount, errorCount, skippedCount
}

// compareMetadata compares comprehensive metadata between source and destination items

// findMetadataDifferences compares two metadata items and returns a list of differences
func (s *SyncOrchestrator) findMetadataDifferences(sourceItem, destItem interface{}, sourceKey, destKey string) []string {
	var differences []string

	// Handle Movie comparison
	if sourceMovie, ok := sourceItem.(plex.Movie); ok {
		if destMovie, ok := destItem.(plex.Movie); ok {
			differences = append(differences, s.compareMovieMetadata(sourceMovie, destMovie)...)
		} else {
			differences = append(differences, "item types differ (source: Movie, dest: not Movie)")
		}
		return differences
	}

	// Handle TVShow comparison
	if sourceTVShow, ok := sourceItem.(plex.TVShow); ok {
		if destTVShow, ok := destItem.(plex.TVShow); ok {
			differences = append(differences, s.compareTVShowMetadata(sourceTVShow, destTVShow)...)
		} else {
			differences = append(differences, "item types differ (source: TVShow, dest: not TVShow)")
		}
		return differences
	}

	differences = append(differences, "unsupported item type for comparison")
	return differences
}

// compareMovieMetadata compares all non-server-specific Movie fields
func (s *SyncOrchestrator) compareMovieMetadata(source, dest plex.Movie) []string {
	var differences []string

	// Compare basic fields
	if source.Title != dest.Title {
		differences = append(differences, fmt.Sprintf("title differs: '%s' vs '%s'", source.Title, dest.Title))
	}
	if source.OriginalTitle != dest.OriginalTitle {
		differences = append(differences, fmt.Sprintf("original title differs: '%s' vs '%s'", source.OriginalTitle, dest.OriginalTitle))
	}
	if source.Year != dest.Year {
		differences = append(differences, fmt.Sprintf("year differs: %d vs %d", source.Year, dest.Year))
	}
	if source.Studio != dest.Studio {
		differences = append(differences, fmt.Sprintf("studio differs: '%s' vs '%s'", source.Studio, dest.Studio))
	}
	if source.ContentRating != dest.ContentRating {
		differences = append(differences, fmt.Sprintf("content rating differs: '%s' vs '%s'", source.ContentRating, dest.ContentRating))
	}
	if source.Summary != dest.Summary {
		differences = append(differences, "summary differs")
	}
	if source.Tagline != dest.Tagline {
		differences = append(differences, fmt.Sprintf("tagline differs: '%s' vs '%s'", source.Tagline, dest.Tagline))
	}

	// Compare ratings (allow small differences due to precision)
	if abs(int64(source.UserRating.Value*10-dest.UserRating.Value*10)) > 1 {
		differences = append(differences, fmt.Sprintf("user rating differs: %.1f vs %.1f", source.UserRating.Value, dest.UserRating.Value))
	}

	// Compare artwork
	if source.Thumb != dest.Thumb {
		differences = append(differences, "poster (thumb) differs")
	}
	if source.Art != dest.Art {
		differences = append(differences, "background (art) differs")
	}

	// Compare arrays
	if !s.compareTagArrays(source.Genre, dest.Genre) {
		differences = append(differences, fmt.Sprintf("genres differ: %v vs %v", s.extractTags(source.Genre), s.extractTags(dest.Genre)))
	}
	if !s.compareTagArrays(source.Label, dest.Label) {
		differences = append(differences, fmt.Sprintf("labels differ: %v vs %v", s.extractTags(source.Label), s.extractTags(dest.Label)))
	}
	if !s.compareCollectionArrays(source.Collection, dest.Collection) {
		differences = append(differences, fmt.Sprintf("collections differ: %v vs %v", s.extractCollectionTags(source.Collection), s.extractCollectionTags(dest.Collection)))
	}

	// Compare watched state
	if source.ViewCount != dest.ViewCount {
		differences = append(differences, fmt.Sprintf("view count differs: %d vs %d", source.ViewCount, dest.ViewCount))
	}

	return differences
}

// compareTVShowMetadata compares all non-server-specific TV Show fields
func (s *SyncOrchestrator) compareTVShowMetadata(source, dest plex.TVShow) []string {
	var differences []string

	// Compare basic fields
	if source.Title != dest.Title {
		differences = append(differences, fmt.Sprintf("title differs: '%s' vs '%s'", source.Title, dest.Title))
	}
	if source.OriginalTitle != dest.OriginalTitle {
		differences = append(differences, fmt.Sprintf("original title differs: '%s' vs '%s'", source.OriginalTitle, dest.OriginalTitle))
	}
	if source.Year != dest.Year {
		differences = append(differences, fmt.Sprintf("year differs: %d vs %d", source.Year, dest.Year))
	}
	if source.Studio != dest.Studio {
		differences = append(differences, fmt.Sprintf("studio differs: '%s' vs '%s'", source.Studio, dest.Studio))
	}
	if source.Network != dest.Network {
		differences = append(differences, fmt.Sprintf("network differs: '%s' vs '%s'", source.Network, dest.Network))
	}
	if source.ContentRating != dest.ContentRating {
		differences = append(differences, fmt.Sprintf("content rating differs: '%s' vs '%s'", source.ContentRating, dest.ContentRating))
	}
	if source.Summary != dest.Summary {
		differences = append(differences, "summary differs")
	}
	if source.Tagline != dest.Tagline {
		differences = append(differences, fmt.Sprintf("tagline differs: '%s' vs '%s'", source.Tagline, dest.Tagline))
	}

	// Compare ratings (allow small differences due to precision)
	if abs(int64(source.UserRating.Value*10-dest.UserRating.Value*10)) > 1 {
		differences = append(differences, fmt.Sprintf("user rating differs: %.1f vs %.1f", source.UserRating.Value, dest.UserRating.Value))
	}

	// Compare artwork
	if source.Thumb != dest.Thumb {
		differences = append(differences, "poster (thumb) differs")
	}
	if source.Art != dest.Art {
		differences = append(differences, "background (art) differs")
	}

	// Compare arrays
	if !s.compareTagArrays(source.Genre, dest.Genre) {
		differences = append(differences, fmt.Sprintf("genres differ: %v vs %v", s.extractTags(source.Genre), s.extractTags(dest.Genre)))
	}
	if !s.compareTagArrays(source.Label, dest.Label) {
		differences = append(differences, fmt.Sprintf("labels differ: %v vs %v", s.extractTags(source.Label), s.extractTags(dest.Label)))
	}
	if !s.compareCollectionArrays(source.Collection, dest.Collection) {
		differences = append(differences, fmt.Sprintf("collections differ: %v vs %v", s.extractCollectionTags(source.Collection), s.extractCollectionTags(dest.Collection)))
	}

	// Compare watched state
	if source.ViewCount != dest.ViewCount {
		differences = append(differences, fmt.Sprintf("view count differs: %d vs %d", source.ViewCount, dest.ViewCount))
	}

	return differences
}

// compareTagArrays compares arrays of tags (Genre/Label)
func (s *SyncOrchestrator) compareTagArrays(source, dest interface{}) bool {
	sourceTags := s.extractTags(source)
	destTags := s.extractTags(dest)

	if len(sourceTags) != len(destTags) {
		return false
	}

	// Convert to maps for easier comparison
	sourceMap := make(map[string]bool)
	destMap := make(map[string]bool)

	for _, tag := range sourceTags {
		sourceMap[tag] = true
	}
	for _, tag := range destTags {
		destMap[tag] = true
	}

	// Check if all source tags exist in dest
	for tag := range sourceMap {
		if !destMap[tag] {
			return false
		}
	}

	return true
}

// compareCollectionArrays compares arrays of collections
func (s *SyncOrchestrator) compareCollectionArrays(source, dest []plex.Collection) bool {
	sourceTags := s.extractCollectionTags(source)
	destTags := s.extractCollectionTags(dest)

	if len(sourceTags) != len(destTags) {
		return false
	}

	// Convert to maps for easier comparison
	sourceMap := make(map[string]bool)
	destMap := make(map[string]bool)

	for _, tag := range sourceTags {
		sourceMap[tag] = true
	}
	for _, tag := range destTags {
		destMap[tag] = true
	}

	// Check if all source tags exist in dest
	for tag := range sourceMap {
		if !destMap[tag] {
			return false
		}
	}

	return true
}

// extractTags extracts tag strings from Genre or Label arrays
func (s *SyncOrchestrator) extractTags(items interface{}) []string {
	var tags []string

	switch v := items.(type) {
	case []plex.Genre:
		for _, item := range v {
			tags = append(tags, item.Tag)
		}
	case []plex.Label:
		for _, item := range v {
			tags = append(tags, item.Tag)
		}
	}

	return tags
}

// extractCollectionTags extracts tag strings from Collection arrays
func (s *SyncOrchestrator) extractCollectionTags(collections []plex.Collection) []string {
	var tags []string
	for _, collection := range collections {
		tags = append(tags, collection.Tag)
	}
	return tags
}

// abs returns the absolute value of an int64
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// TODO: Uncomment when plexgo library implements complete metadata sync functions
// func (s *SyncOrchestrator) syncItemMetadata(match discovery.ItemMatch) error {
//     sourceItem := match.SourceItem
//     destRatingKey := s.getDestRatingKey(match.DestItem)
//
//     // Sync all metadata fields using plexgo library functions
//     if err := s.syncBasicMetadata(sourceItem, destRatingKey); err != nil {
//         return err
//     }
//
//     // Sync user ratings
//     if err := s.sourceClient.SetUserRating(destRatingKey, sourceItem.UserRating); err != nil {
//         return err
//     }
//
//     // Sync selected poster
//     if err := s.syncPoster(sourceItem, destRatingKey); err != nil {
//         return err
//     }
//
//     // Sync custom titles and names
//     if err := s.syncCustomFields(sourceItem, destRatingKey); err != nil {
//         return err
//     }
//
//     // Sync all labels
//     if err := s.sourceClient.SetItemLabels(destRatingKey, sourceItem.Labels); err != nil {
//         return err
//     }
//
//     // Sync watched state
//     if err := s.syncWatchedState(sourceItem, destRatingKey); err != nil {
//         return err
//     }
//
//     return nil
// }

// RunContinuous runs the sync process in a continuous loop
func (s *SyncOrchestrator) RunContinuous() error {
	s.logger.WithField("interval", s.config.Interval.String()).Info("Starting continuous sync mode")

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	// Run initial sync
	if err := s.RunSyncCycle(); err != nil {
		s.logger.WithError(err).Error("Initial sync cycle failed")
	}

	// Run periodic syncs
	for range ticker.C {
		if err := s.RunSyncCycle(); err != nil {
			s.logger.WithError(err).Error("Sync cycle failed")
		}
	}

	return nil
}

// HandleForceFullSync clears all sync state and forces a complete re-sync
func (s *SyncOrchestrator) HandleForceFullSync() error {
	if !s.config.ForceFullSync {
		return nil
	}

	s.logger.Info("Force full sync enabled - will perform complete synchronization")

	// TODO: Clear sync state from database/storage when state management is implemented
	s.logger.Info("Sync state cleared for force full sync")

	return nil
}

// Helper methods for Enhanced Media Items

// getEnhancedItemTitle safely extracts title from an enhanced media item
func (s *SyncOrchestrator) getEnhancedItemTitle(enhancedItem *discovery.EnhancedMediaItem) string {
	return s.getItemTitle(enhancedItem.Item)
}

// getEnhancedItemRatingKey safely extracts rating key from an enhanced media item
func (s *SyncOrchestrator) getEnhancedItemRatingKey(enhancedItem *discovery.EnhancedMediaItem) string {
	return s.getItemRatingKey(enhancedItem.Item)
}

// compareEnhancedMetadata compares metadata between enhanced source and destination items
func (s *SyncOrchestrator) compareEnhancedMetadata(sourceEnhanced, destEnhanced *discovery.EnhancedMediaItem) (bool, error) {
	// Now we have FULL metadata for both items, so we can do direct comparison
	differences := s.findEnhancedMetadataDifferences(sourceEnhanced, destEnhanced)

	if len(differences) > 0 {
		s.logger.WithFields(map[string]interface{}{
			"source_key":  s.getEnhancedItemRatingKey(sourceEnhanced),
			"dest_key":    s.getEnhancedItemRatingKey(destEnhanced),
			"differences": differences,
		}).Debug("Enhanced metadata differences found")
		return true, nil
	}

	s.logger.WithFields(map[string]interface{}{
		"source_key": s.getEnhancedItemRatingKey(sourceEnhanced),
		"dest_key":   s.getEnhancedItemRatingKey(destEnhanced),
	}).Debug("Enhanced metadata is synchronized")

	return false, nil
}

// findEnhancedMetadataDifferences compares two enhanced metadata items and returns differences
func (s *SyncOrchestrator) findEnhancedMetadataDifferences(sourceEnhanced, destEnhanced *discovery.EnhancedMediaItem) []string {
	// Direct comparison using full metadata
	return s.findMetadataDifferences(sourceEnhanced.Item, destEnhanced.Item,
		s.getEnhancedItemRatingKey(sourceEnhanced), s.getEnhancedItemRatingKey(destEnhanced))
}

// Legacy Helper methods (for backward compatibility)

// getItemTitle safely extracts title from an item
func (s *SyncOrchestrator) getItemTitle(item interface{}) string {
	switch v := item.(type) {
	case plex.Movie:
		return v.Title
	case plex.TVShow:
		return v.Title
	default:
		return "unknown"
	}
}

// getItemRatingKey safely extracts rating key from an item
func (s *SyncOrchestrator) getItemRatingKey(item interface{}) string {
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

// isSSHConfigured checks if SSH is properly configured for username/password authentication
func isSSHConfigured(sshConfig config.SSHConfig, log *logger.Logger) bool {
	// Check if SSH user and password are provided
	if sshConfig.User == "" || sshConfig.Password == "" {
		log.Debug("SSH user or password not provided")
		return false
	}

	// Check for common placeholder values (but not "nullable" since that's a real username)
	if sshConfig.User == "your-ssh-username" ||
		sshConfig.Password == "your-ssh-password" {
		log.Info("SSH configuration contains placeholder values - skipping SSH setup")
		return false
	}

	log.WithFields(map[string]interface{}{
		"ssh_user": sshConfig.User,
		"ssh_port": sshConfig.Port,
	}).Debug("SSH configured for username/password authentication")

	return true
}
