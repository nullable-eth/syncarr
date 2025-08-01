package plex

import (
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
)

// Client represents a Plex API client
type Client struct {
	config     *config.PlexServerConfig
	logger     *logger.Logger
	httpClient *http.Client
}

// NewClient creates a new Plex client
func NewClient(cfg *config.PlexServerConfig, log *logger.Logger) (*Client, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &Client{
		config:     cfg,
		logger:     log,
		httpClient: &http.Client{Transport: tr},
	}

	// Test the connection
	if err := client.TestConnection(); err != nil {
		return nil, fmt.Errorf("failed to connect to Plex server: %w", err)
	}

	client.logger.WithFields(map[string]interface{}{
		"server_url": client.buildURL(""),
		"host":       cfg.Host,
		"port":       cfg.Port,
		"https":      cfg.RequireHTTPS,
	}).Info("Plex client created successfully")

	return client, nil
}

// TestConnection tests if the Plex server is reachable by hitting the /identity endpoint
func (c *Client) TestConnection() error {
	url := c.buildURL("/identity")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Plex server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plex server returned status %d", resp.StatusCode)
	}

	c.logger.WithField("server_url", c.config.Host).Debug("Successfully connected to Plex server")
	return nil
}

// GetLibraries fetches all libraries from Plex
func (c *Client) GetLibraries() ([]Library, error) {
	librariesURL := c.buildURL("/library/sections")

	req, err := http.NewRequest("GET", librariesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch libraries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plex API returned status %d. Response: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var libraryResponse LibraryResponse
	if err := json.Unmarshal(body, &libraryResponse); err != nil {
		return nil, fmt.Errorf("failed to parse library response: %w. Response body: %s", err, string(body))
	}

	return libraryResponse.MediaContainer.Directory, nil
}

// GetMoviesFromLibrary fetches all movies from a specific library with detailed metadata including labels
func (c *Client) GetMoviesFromLibrary(libraryID string) ([]Movie, error) {
	moviesURL := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	req, err := http.NewRequest("GET", moviesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch movies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var plexResponse PlexResponse
	if err := json.Unmarshal(body, &plexResponse); err != nil {
		return nil, fmt.Errorf("failed to parse movies response: %w", err)
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id": libraryID,
		"item_count": len(plexResponse.MediaContainer.Metadata),
	}).Info("Retrieved basic movie metadata, fetching detailed metadata for labels")

	c.logger.WithFields(map[string]interface{}{
		"library_id":  libraryID,
		"movie_count": len(plexResponse.MediaContainer.Metadata),
	}).Debug("Retrieved movies from library")

	return plexResponse.MediaContainer.Metadata, nil
}

// GetTVShowsFromLibrary fetches all TV shows from a specific library
func (c *Client) GetTVShowsFromLibrary(libraryID string) ([]TVShow, error) {
	tvShowsURL := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	req, err := http.NewRequest("GET", tvShowsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TV shows: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var tvShowResponse TVShowResponse
	if err := json.Unmarshal(body, &tvShowResponse); err != nil {
		return nil, fmt.Errorf("failed to parse TV shows response: %w", err)
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id": libraryID,
		"item_count": len(tvShowResponse.MediaContainer.Metadata),
	}).Info("Retrieved TV shows from library")

	return tvShowResponse.MediaContainer.Metadata, nil
}

// GetAllTVShowEpisodes fetches ALL episodes for a specific TV show
func (c *Client) GetAllTVShowEpisodes(ratingKey string) ([]Episode, error) {
	episodesURL := c.buildURL(fmt.Sprintf("/library/metadata/%s/allLeaves", ratingKey))

	req, err := http.NewRequest("GET", episodesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all TV show episodes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var episodeResponse EpisodeResponse
	if err := json.Unmarshal(body, &episodeResponse); err != nil {
		return nil, fmt.Errorf("failed to parse episodes response: %w", err)
	}

	return episodeResponse.MediaContainer.Metadata, nil
}

// UpdateMediaField updates a media item's field (labels or genres) with new keywords
func (c *Client) UpdateMediaField(mediaID, libraryID string, keywords []string, updateField string, mediaType string) error {
	c.logger.WithFields(map[string]interface{}{
		"media_id":      mediaID,
		"library_id":    libraryID,
		"update_field":  updateField,
		"keyword_count": len(keywords),
	}).Debug("Making Plex API call to update media field")

	return c.updateMediaField(mediaID, libraryID, keywords, updateField, c.getMediaTypeForLibraryType(mediaType))
}

// RemoveMediaFieldKeywords removes keywords from a media item's field
func (c *Client) RemoveMediaFieldKeywords(mediaID, libraryID string, valuesToRemove []string, updateField string, lockField bool, mediaType string) error {
	return c.removeMediaFieldKeywords(mediaID, libraryID, valuesToRemove, updateField, lockField, c.getMediaTypeForLibraryType(mediaType))
}

// TriggerLibraryScan triggers a scan of the specified library
func (c *Client) TriggerLibraryScan(libraryID string) error {
	url := c.buildURL(fmt.Sprintf("/library/sections/%s/refresh", libraryID))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to trigger library scan: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to trigger library scan, status code: %d", resp.StatusCode)
	}

	c.logger.WithField("library_id", libraryID).Debug("Triggered library scan")
	return nil
}

// TriggerMetadataRefresh triggers a full metadata refresh for the specified library
func (c *Client) TriggerMetadataRefresh(libraryID string) error {
	url := c.buildURL(fmt.Sprintf("/library/sections/%s/refresh?force=1", libraryID))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to trigger metadata refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to trigger metadata refresh, status code: %d", resp.StatusCode)
	}

	c.logger.WithField("library_id", libraryID).Debug("Triggered metadata refresh")
	return nil
}

// GetActivities retrieves all current activities from the Plex server
func (c *Client) GetActivities() (*ActivitiesResponse, error) {
	url := c.buildURL("/activities")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get activities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get activities, status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var activitiesResponse ActivitiesResponse
	if err := xml.Unmarshal(body, &activitiesResponse); err != nil {
		return nil, fmt.Errorf("failed to parse activities response: %w", err)
	}

	c.logger.WithFields(map[string]interface{}{
		"activity_count": activitiesResponse.Size,
		"activities":     len(activitiesResponse.Activities),
	}).Debug("Retrieved server activities")

	return &activitiesResponse, nil
}

// IsLibraryScanInProgress checks if any library scans are currently running
func (c *Client) IsLibraryScanInProgress() (bool, []Activity, error) {
	activities, err := c.GetActivities()
	if err != nil {
		return false, nil, err
	}

	var libraryScanActivities []Activity
	for _, activity := range activities.Activities {
		if activity.Type == "library.update.section" {
			libraryScanActivities = append(libraryScanActivities, activity)
		}
	}

	return len(libraryScanActivities) > 0, libraryScanActivities, nil
}

// GetWatchedState retrieves the watched state for a media item
func (c *Client) GetWatchedState(ratingKey string) (*WatchedState, error) {
	url := c.buildURL(fmt.Sprintf("/library/metadata/%s", ratingKey))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get media metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get media metadata, status code: %d", resp.StatusCode)
	}

	// For now, return default state - TODO: Parse actual response
	watchedState := &WatchedState{
		Watched:      false,
		ViewCount:    0,
		ViewOffset:   0,
		LastViewedAt: 0,
	}

	c.logger.WithField("rating_key", ratingKey).Debug("Retrieved watched state (parsing not yet implemented)")
	return watchedState, nil
}

// SetWatchedState sets the watched state for a media item
func (c *Client) SetWatchedState(ratingKey string, watched bool) error {
	var endpoint string
	if watched {
		endpoint = "/:/scrobble"
	} else {
		endpoint = "/:/unscrobble"
	}

	urlStr := c.buildURL(endpoint)

	// Parse the URL to add query parameters
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	params := parsedURL.Query()
	params.Set("key", ratingKey)
	params.Set("identifier", "com.plexapp.plugins.library")
	params.Set("X-Plex-Token", c.config.Token)
	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set watched state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set watched state, status code: %d", resp.StatusCode)
	}

	c.logger.WithFields(map[string]interface{}{
		"rating_key": ratingKey,
		"watched":    watched,
	}).Debug("Set watched state")

	return nil
}

// SetUserRating sets the user rating for a media item (0.0 to 10.0)
func (c *Client) SetUserRating(ratingKey string, rating float64) error {
	if rating < 0 || rating > 10 {
		return fmt.Errorf("rating must be between 0 and 10, got %.1f", rating)
	}

	urlStr := c.buildURL("/:/rate")
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	params := parsedURL.Query()
	params.Set("key", ratingKey)
	params.Set("rating", fmt.Sprintf("%.0f", rating))
	params.Set("identifier", "com.plexapp.plugins.library")
	params.Set("X-Plex-Token", c.config.Token)
	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set user rating: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to set user rating, status code: %d", resp.StatusCode)
	}

	c.logger.WithFields(map[string]interface{}{
		"rating_key": ratingKey,
		"rating":     rating,
	}).Debug("Set user rating")

	return nil
}

// SetLabels sets labels for a media item
func (c *Client) SetLabels(ratingKey, libraryID string, labels []string) error {
	return c.UpdateMediaField(ratingKey, libraryID, labels, "label", "movie")
}

// SetTitle sets the title for a media item
func (c *Client) SetTitle(ratingKey, libraryID, title string) error {
	return c.updateBasicField(ratingKey, libraryID, "title", title)
}

// SetSummary sets the summary for a media item
func (c *Client) SetSummary(ratingKey, libraryID, summary string) error {
	return c.updateBasicField(ratingKey, libraryID, "summary", summary)
}

// updateBasicField updates basic text fields like title, summary, etc.
func (c *Client) updateBasicField(ratingKey, libraryID, fieldName, value string) error {
	baseURL := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	params := parsedURL.Query()
	params.Set("type", "1") // Assume movie for now, could be enhanced
	params.Set("id", ratingKey)
	params.Set(fieldName, value)
	params.Set("X-Plex-Token", c.config.Token)
	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("PUT", parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", fieldName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update %s, status code: %d - Response: %s", fieldName, resp.StatusCode, string(body))
	}

	c.logger.WithFields(map[string]interface{}{
		"rating_key": ratingKey,
		"field":      fieldName,
		"value":      value,
	}).Debug("Updated basic field")

	return nil
}

// Helper Methods

// updateMediaField is a generic function to update media fields (movies: type=1, TV shows: type=2)
func (c *Client) updateMediaField(mediaID, libraryID string, keywords []string, updateField string, mediaType int) error {
	startTime := time.Now()

	// Build the base URL
	baseURL := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	// Parse the URL to add query parameters properly
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create query parameters
	params := parsedURL.Query()
	params.Set("type", fmt.Sprintf("%d", mediaType))
	params.Set("id", mediaID)
	params.Set("includeExternalMedia", "1")

	// Add indexed label/genre parameters like label[0].tag.tag, label[1].tag.tag, etc.
	for i, keyword := range keywords {
		paramName := fmt.Sprintf("%s[%d].tag.tag", updateField, i)
		params.Set(paramName, keyword)
	}

	params.Set(fmt.Sprintf("%s.locked", updateField), "1")
	params.Set("X-Plex-Token", c.config.Token)

	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("PUT", parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update media field: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plex API returned status %d when updating media field - Response: %s", resp.StatusCode, string(body))
	}

	duration := time.Since(startTime)
	c.logger.WithField("duration", duration).Debug("Plex API call completed")

	return nil
}

// removeMediaFieldKeywords is a generic function to remove keywords from media fields (movies: type=1, TV shows: type=2)
func (c *Client) removeMediaFieldKeywords(mediaID, libraryID string, valuesToRemove []string, updateField string, lockField bool, mediaType int) error {
	// Build the base URL
	baseURL := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	// Parse the URL to add query parameters properly
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create query parameters
	params := parsedURL.Query()
	params.Set("type", fmt.Sprintf("%d", mediaType))
	params.Set("id", mediaID)
	params.Set("includeExternalMedia", "1")

	// Join values with commas for the -= operator
	combinedValues := strings.Join(valuesToRemove, ",")

	// Add removal parameter using the -= operator
	paramName := fmt.Sprintf("%s[].tag.tag-", updateField)
	params.Set(paramName, combinedValues)

	if lockField {
		params.Set(fmt.Sprintf("%s.locked", updateField), "1")
	} else {
		params.Set(fmt.Sprintf("%s.locked", updateField), "0")
	}
	params.Set("X-Plex-Token", c.config.Token)

	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("PUT", parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove media field keywords: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plex API returned status %d when removing media field keywords - Response: %s", resp.StatusCode, string(body))
	}

	return nil
}

// getMediaTypeForLibraryType converts library type strings to Plex API media type integers
func (c *Client) getMediaTypeForLibraryType(libraryType string) int {
	switch libraryType {
	case "movie":
		return 1
	case "show":
		return 2
	default:
		// Default to 1 for unknown types
		return 1
	}
}

// buildURL constructs a full URL for Plex API requests
func (c *Client) buildURL(path string) string {
	protocol := "http"
	if c.config.RequireHTTPS {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s:%s%s", protocol, c.config.Host, c.config.Port, path)
}

// GetLibraryContent retrieves all content from a specific library (movies and TV shows)
func (c *Client) GetLibraryContent(libraryID string) ([]interface{}, error) {
	libraries, err := c.GetLibraries()
	if err != nil {
		return nil, fmt.Errorf("failed to get libraries: %w", err)
	}

	// Find the library type
	var libraryType string
	for _, lib := range libraries {
		if lib.Key == libraryID {
			libraryType = lib.Type
			break
		}
	}

	var allItems []interface{}

	if libraryType == "movie" {
		movies, err := c.GetMoviesFromLibrary(libraryID)
		if err != nil {
			return nil, err
		}
		for _, movie := range movies {
			allItems = append(allItems, movie)
		}
	} else if libraryType == "show" {
		shows, err := c.GetTVShowsFromLibrary(libraryID)
		if err != nil {
			return nil, err
		}
		for _, show := range shows {
			allItems = append(allItems, show)
		}
	} else {
		// Try both types for unknown library types
		movies, err := c.GetMoviesFromLibrary(libraryID)
		if err == nil {
			for _, movie := range movies {
				allItems = append(allItems, movie)
			}
		}
		shows, err := c.GetTVShowsFromLibrary(libraryID)
		if err == nil {
			for _, show := range shows {
				allItems = append(allItems, show)
			}
		}
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id":   libraryID,
		"item_count":   len(allItems),
		"library_type": libraryType,
	}).Info("Retrieved library content")

	return allItems, nil
}

// GetItemsWithLabelDirect efficiently retrieves items with a specific label using server-side filtering
// and then fetches detailed metadata including labels for each item
func (c *Client) GetItemsWithLabelDirect(libraryID, label string) ([]interface{}, error) {
	// Use Plex API query parameters to filter by label server-side
	// This is much more efficient than downloading all items and filtering client-side
	url := c.buildURL(fmt.Sprintf("/library/sections/%s/all", libraryID))

	// Add label filter query parameter (based on Python PlexAPI approach)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add query parameters for label filtering
	q := req.URL.Query()
	q.Add("label", label) // Server-side label filtering
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	var result struct {
		MediaContainer struct {
			Metadata []json.RawMessage `json:"Metadata"`
		} `json:"MediaContainer"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id":     libraryID,
		"label":          label,
		"filtered_items": len(result.MediaContainer.Metadata),
	}).Debug("Got basic items with label filter, fetching detailed metadata")

	var items []interface{}
	for _, rawItem := range result.MediaContainer.Metadata {
		var basicItem struct {
			Type      string `json:"type"`
			RatingKey string `json:"ratingKey"`
			Title     string `json:"title"`
		}
		if err := json.Unmarshal(rawItem, &basicItem); err != nil {
			c.logger.WithError(err).Warn("Failed to parse basic item info")
			continue
		}

		switch basicItem.Type {
		case "movie":
			// Get detailed movie metadata including labels
			detailedMovie, err := c.GetMovieDetails(basicItem.RatingKey)
			if err != nil {
				c.logger.WithError(err).WithFields(map[string]interface{}{
					"rating_key": basicItem.RatingKey,
					"title":      basicItem.Title,
				}).Warn("Failed to fetch detailed metadata, using basic metadata")
				continue
			}

			c.logger.WithFields(map[string]interface{}{
				"rating_key": detailedMovie.RatingKey.String(),
				"title":      detailedMovie.Title,
			}).Debug("Successfully fetched detailed movie metadata")

			items = append(items, *detailedMovie)

		case "show":
			// Get detailed TV show metadata including labels
			detailedShow, err := c.GetTVShowDetails(basicItem.RatingKey)
			if err != nil {
				c.logger.WithError(err).WithFields(map[string]interface{}{
					"rating_key": basicItem.RatingKey,
					"title":      basicItem.Title,
				}).Warn("Failed to fetch detailed show metadata, using basic metadata")
				continue
			}

			// Get all episodes for this TV show
			episodes, err := c.GetAllTVShowEpisodes(basicItem.RatingKey)
			if err != nil {
				c.logger.WithError(err).WithFields(map[string]interface{}{
					"rating_key": basicItem.RatingKey,
					"title":      basicItem.Title,
				}).Warn("Failed to get TV show episodes, adding show without episodes")
				items = append(items, *detailedShow)
				continue
			}

			c.logger.WithFields(map[string]interface{}{
				"show_title":    detailedShow.Title,
				"rating_key":    detailedShow.RatingKey.String(),
				"episode_count": len(episodes),
			}).Debug("Successfully fetched detailed show metadata with episodes")

			// Add the detailed show
			items = append(items, *detailedShow)
			c.logger.WithFields(map[string]interface{}{
				"show_title":    detailedShow.Title,
				"rating_key":    detailedShow.RatingKey.String(),
				"label_count":   len(detailedShow.Label),
				"episode_count": len(episodes),
			}).Debug("Added detailed TV show metadata with episodes")

			// Add all episodes
			for _, episode := range episodes {
				items = append(items, episode)
			}
		}
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id":  libraryID,
		"label":       label,
		"total_items": len(items),
	}).Debug("Completed detailed metadata fetch for labeled items")

	return items, nil
}

// GetItemsWithLabel now uses the more efficient server-side filtering
func (c *Client) GetItemsWithLabel(libraryID, label string) ([]interface{}, error) {
	c.logger.WithFields(map[string]interface{}{
		"library_id": libraryID,
		"label":      label,
	}).Debug("Getting items with label using server-side filtering")

	// Try the efficient server-side filtering first
	items, err := c.GetItemsWithLabelDirect(libraryID, label)
	if err != nil {
		c.logger.WithError(err).WithFields(map[string]interface{}{
			"library_id": libraryID,
			"label":      label,
		}).Warn("Server-side filtering failed, falling back to client-side")
		// Fallback to client-side filtering if server-side fails
		return c.GetItemsWithLabelClientSide(libraryID, label)
	}

	c.logger.WithFields(map[string]interface{}{
		"library_id": libraryID,
		"label":      label,
		"item_count": len(items),
	}).Debug("Found items with label using server-side filtering")
	return items, nil
}

// GetItemsWithLabelClientSide provides fallback client-side filtering
func (c *Client) GetItemsWithLabelClientSide(libraryID, label string) ([]interface{}, error) {
	// Get all content from the library
	allItems, err := c.GetLibraryContent(libraryID)
	if err != nil {
		return nil, err
	}

	// Filter items that have the specified label (client-side)
	var filteredItems []interface{}
	for _, item := range allItems {
		if c.itemHasLabel(item, label) {
			filteredItems = append(filteredItems, item)
		}
	}

	return filteredItems, nil
}

// itemHasLabel checks if an item has a specific label
func (c *Client) itemHasLabel(item interface{}, label string) bool {
	switch v := item.(type) {
	case Movie:
		for _, lbl := range v.Label {
			if lbl.Tag == label {
				return true
			}
		}
	case TVShow:
		for _, lbl := range v.Label {
			if lbl.Tag == label {
				return true
			}
		}
	}
	return false
}

// GetMovieDetails fetches detailed metadata for a specific movie including labels
func (c *Client) GetMovieDetails(ratingKey string) (*Movie, error) {
	url := c.buildURL(fmt.Sprintf("/library/metadata/%s", ratingKey))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch movie details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var movieResponse PlexResponse
	if err := json.Unmarshal(body, &movieResponse); err != nil {
		return nil, fmt.Errorf("failed to parse movie details response: %w", err)
	}

	if len(movieResponse.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("no movie found with rating key %s", ratingKey)
	}

	return &movieResponse.MediaContainer.Metadata[0], nil
}

// GetTVShowDetails fetches detailed metadata for a specific TV show including labels
func (c *Client) GetTVShowDetails(ratingKey string) (*TVShow, error) {
	url := c.buildURL(fmt.Sprintf("/library/metadata/%s", ratingKey))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Plex-Token", c.config.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TV show details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var tvShowResponse TVShowResponse
	if err := json.Unmarshal(body, &tvShowResponse); err != nil {
		return nil, fmt.Errorf("failed to parse TV show details response: %w", err)
	}

	if len(tvShowResponse.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("no TV show found with rating key %s", ratingKey)
	}

	return &tvShowResponse.MediaContainer.Metadata[0], nil
}
