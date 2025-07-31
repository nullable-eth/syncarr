package discovery

import (
	"fmt"
	"time"

	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/plex"
)

// LibraryManager handles Phase 4: Library refresh and monitoring
type LibraryManager struct {
	destClient *plex.Client
	logger     *logger.Logger
}

// NewLibraryManager creates a new library manager
func NewLibraryManager(destClient *plex.Client, log *logger.Logger) *LibraryManager {
	return &LibraryManager{
		destClient: destClient,
		logger:     log,
	}
}

// TriggerRefreshAndWait triggers library scans and waits for completion
func (lm *LibraryManager) TriggerRefreshAndWait() error {
	lm.logger.Info("Phase 4: Triggering library refresh on destination server")

	// First, wait for any existing scans to complete before starting new ones
	lm.logger.Debug("Checking for existing library scans before starting new ones")
	if err := lm.waitForExistingScansComplete(); err != nil {
		lm.logger.WithError(err).Warn("Failed to wait for existing scans, proceeding anyway")
	}

	// Get all destination libraries
	libraries, err := lm.destClient.GetLibraries()
	if err != nil {
		return fmt.Errorf("failed to get destination libraries: %w", err)
	}

	lm.logger.WithField("library_count", len(libraries)).Info("Triggering scans for all libraries")

	// Track which libraries we successfully triggered scans for
	var successfulScans []plex.Library
	var failedScans []string

	// Trigger scan for each library
	for _, library := range libraries {
		lm.logger.WithFields(map[string]interface{}{
			"library_id":    library.Key,
			"library_title": library.Title,
			"library_type":  library.Type,
		}).Debug("Triggering library scan")

		if err := lm.destClient.TriggerLibraryScan(library.Key); err != nil {
			lm.logger.WithError(err).WithFields(map[string]interface{}{
				"library_id":    library.Key,
				"library_title": library.Title,
			}).Error("Failed to trigger library scan")
			failedScans = append(failedScans, library.Title)
			continue
		}

		successfulScans = append(successfulScans, library)
	}

	// Log summary of scan triggers
	lm.logger.WithFields(map[string]interface{}{
		"successful_scans": len(successfulScans),
		"failed_scans":     len(failedScans),
		"total_libraries":  len(libraries),
	}).Info("Library scan trigger summary")

	if len(failedScans) > 0 {
		lm.logger.WithField("failed_libraries", failedScans).Warn("Some library scans failed to trigger")
	}

	// If no scans were successfully triggered, don't wait
	if len(successfulScans) == 0 {
		return fmt.Errorf("failed to trigger any library scans")
	}

	// Monitor scan completion for successfully triggered scans
	return lm.waitForAllScansComplete(successfulScans)
}

// waitForExistingScansComplete waits for any existing library scans to complete
func (lm *LibraryManager) waitForExistingScansComplete() error {
	lm.logger.Debug("Checking for existing library scan activities")

	scanInProgress, activities, err := lm.destClient.IsLibraryScanInProgress()
	if err != nil {
		return fmt.Errorf("failed to check existing scan status: %w", err)
	}

	if !scanInProgress {
		lm.logger.Debug("No existing library scans in progress")
		return nil
	}

	lm.logger.WithField("active_scans", len(activities)).Info("Waiting for existing library scans to complete before starting new ones")

	const maxExistingWaitTime = 5 * time.Minute
	startTime := time.Now()

	for {
		if time.Since(startTime) > maxExistingWaitTime {
			lm.logger.Warn("Timed out waiting for existing scans to complete, proceeding anyway")
			return nil
		}

		scanInProgress, activities, err := lm.destClient.IsLibraryScanInProgress()
		if err != nil {
			lm.logger.WithError(err).Warn("Error checking existing scan status")
			return nil
		}

		if !scanInProgress {
			lm.logger.Info("Existing library scans completed")
			return nil
		}

		lm.logger.WithField("remaining_scans", len(activities)).Debug("Still waiting for existing scans to complete")
		time.Sleep(10 * time.Second)
	}
}

// waitForAllScansComplete monitors all library scans until completion
func (lm *LibraryManager) waitForAllScansComplete(libraries []plex.Library) error {
	lm.logger.Info("Monitoring library scan completion using Plex activities API")

	const (
		pollInterval    = 5 * time.Second  // Check every 5 seconds
		maxWaitTime     = 10 * time.Minute // Maximum wait time
		progressLogTime = 30 * time.Second // Log progress every 30 seconds
	)

	startTime := time.Now()
	lastProgressLog := time.Now()

	for {
		// Check if we've exceeded maximum wait time
		if time.Since(startTime) > maxWaitTime {
			lm.logger.WithField("max_wait_time", maxWaitTime).Warn("Library scan monitoring timed out")
			return fmt.Errorf("library scan monitoring timed out after %v", maxWaitTime)
		}

		// Check if any library scans are still in progress
		scanInProgress, activities, err := lm.destClient.IsLibraryScanInProgress()
		if err != nil {
			lm.logger.WithError(err).Warn("Failed to check library scan status, continuing to wait")
			time.Sleep(pollInterval)
			continue
		}

		// If no scans in progress, we're done
		if !scanInProgress {
			duration := time.Since(startTime)
			lm.logger.WithFields(map[string]interface{}{
				"total_duration": duration,
				"library_count":  len(libraries),
			}).Info("All library scans completed successfully")
			return nil
		}

		// Log progress periodically at INFO level, but log individual checks at DEBUG
		if time.Since(lastProgressLog) >= progressLogTime {
			lm.logger.WithFields(map[string]interface{}{
				"active_scans": len(activities),
				"elapsed":      time.Since(startTime).Round(time.Second),
			}).Info("Library scans still in progress")
			lm.logScanProgress(activities)
			lastProgressLog = time.Now()
		} else {
			// Log individual checks at DEBUG level
			lm.logger.WithFields(map[string]interface{}{
				"active_scans": len(activities),
				"elapsed":      time.Since(startTime).Round(time.Second),
			}).Debug("Checking library scan status")
		}

		// Wait before next check
		time.Sleep(pollInterval)
	}
}

// logScanProgress logs the current progress of library scans
func (lm *LibraryManager) logScanProgress(activities []plex.Activity) {
	if len(activities) == 0 {
		return
	}

	lm.logger.WithField("active_scans", len(activities)).Debug("Library scans in progress...")

	for _, activity := range activities {
		fields := map[string]interface{}{
			"activity_uuid": activity.UUID,
			"title":         activity.Title,
			"progress":      fmt.Sprintf("%d%%", activity.Progress),
		}

		// Add library ID if context is available
		if activity.Context != nil && activity.Context.LibrarySectionID != "" {
			fields["library_id"] = activity.Context.LibrarySectionID
		}

		if activity.Subtitle != "" {
			fields["subtitle"] = activity.Subtitle
		}

		lm.logger.WithFields(fields).Debug("Scan progress")
	}
}
