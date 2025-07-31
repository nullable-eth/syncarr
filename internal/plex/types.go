package plex

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
)

// Library represents a Plex library
type Library struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Agent string `json:"agent"`
}

// LibraryContainer holds library directory information
type LibraryContainer struct {
	Size      int       `json:"size"`
	Directory []Library `json:"Directory"`
}

// LibraryResponse represents the response from library endpoints
type LibraryResponse struct {
	MediaContainer LibraryContainer `json:"MediaContainer"`
}

// Movie represents a Plex movie
type Movie struct {
	RatingKey                     FlexibleRatingKey `json:"ratingKey"`
	Title                         string            `json:"title"`
	TitleSort                     string            `json:"titleSort,omitempty"`
	OriginalTitle                 string            `json:"originalTitle,omitempty"`
	Year                          int               `json:"year"`
	Duration                      int               `json:"duration,omitempty"`
	ContentRating                 string            `json:"contentRating,omitempty"`
	Studio                        string            `json:"studio,omitempty"`
	Tagline                       string            `json:"tagline,omitempty"`
	Summary                       string            `json:"summary,omitempty"`
	Rating                        FlexibleRating    `json:"rating,omitempty"`
	AudienceRating                FlexibleRating    `json:"audienceRating,omitempty"`
	RatingImage                   string            `json:"ratingImage,omitempty"`
	AudienceRatingImage           string            `json:"audienceRatingImage,omitempty"`
	ViewCount                     int               `json:"viewCount,omitempty"`
	LastViewedAt                  int               `json:"lastViewedAt,omitempty"`
	ViewOffset                    int               `json:"viewOffset,omitempty"`
	UserRating                    FlexibleRating    `json:"userRating,omitempty"`
	OriginallyAvailableAt         string            `json:"originallyAvailableAt,omitempty"`
	AddedAt                       int               `json:"addedAt,omitempty"`
	UpdatedAt                     int               `json:"updatedAt,omitempty"`
	Thumb                         string            `json:"thumb,omitempty"`
	Art                           string            `json:"art,omitempty"`
	Theme                         string            `json:"theme,omitempty"`
	ChapterSource                 string            `json:"chapterSource,omitempty"`
	PrimaryExtraKey               string            `json:"primaryExtraKey,omitempty"`
	EditionTitle                  string            `json:"editionTitle,omitempty"`
	EnableCreditsMarkerGeneration int               `json:"enableCreditsMarkerGeneration,omitempty"`
	LanguageOverride              string            `json:"languageOverride,omitempty"`
	UseOriginalTitle              int               `json:"useOriginalTitle,omitempty"`
	Slug                          string            `json:"slug,omitempty"`
	SourceURI                     string            `json:"sourceURI,omitempty"`
	Label                         []Label           `json:"Label,omitempty"`
	Genre                         []Genre           `json:"Genre,omitempty"`
	Director                      []Director        `json:"Director,omitempty"`
	Writer                        []Writer          `json:"Writer,omitempty"`
	Producer                      []Producer        `json:"Producer,omitempty"`
	Role                          []Role            `json:"Role,omitempty"`
	Country                       []Country         `json:"Country,omitempty"`
	Collection                    []Collection      `json:"Collection,omitempty"`
	Guid                          FlexibleGuid      `json:"Guid,omitempty"`
	Media                         []Media           `json:"Media,omitempty"`
}

// MediaItem interface implementation for Movie
func (m Movie) GetRatingKey() string { return m.RatingKey.String() }
func (m Movie) GetTitle() string     { return m.Title }
func (m Movie) GetYear() int         { return m.Year }
func (m Movie) GetGuid() []Guid      { return []Guid(m.Guid) }
func (m Movie) GetMedia() []Media    { return m.Media }
func (m Movie) GetLabel() []Label    { return m.Label }
func (m Movie) GetGenre() []Genre    { return m.Genre }

// TVShow represents a Plex TV show
type TVShow struct {
	RatingKey                              FlexibleRatingKey `json:"ratingKey"`
	Title                                  string            `json:"title"`
	TitleSort                              string            `json:"titleSort,omitempty"`
	OriginalTitle                          string            `json:"originalTitle,omitempty"`
	Year                                   int               `json:"year"`
	Duration                               int               `json:"duration,omitempty"`
	ContentRating                          string            `json:"contentRating,omitempty"`
	Studio                                 string            `json:"studio,omitempty"`
	Network                                string            `json:"network,omitempty"`
	Tagline                                string            `json:"tagline,omitempty"`
	Summary                                string            `json:"summary,omitempty"`
	Rating                                 FlexibleRating    `json:"rating,omitempty"`
	AudienceRating                         FlexibleRating    `json:"audienceRating,omitempty"`
	RatingImage                            string            `json:"ratingImage,omitempty"`
	AudienceRatingImage                    string            `json:"audienceRatingImage,omitempty"`
	ViewCount                              int               `json:"viewCount,omitempty"`
	LastViewedAt                           int               `json:"lastViewedAt,omitempty"`
	ViewOffset                             int               `json:"viewOffset,omitempty"`
	UserRating                             FlexibleRating    `json:"userRating,omitempty"`
	OriginallyAvailableAt                  string            `json:"originallyAvailableAt,omitempty"`
	AddedAt                                int               `json:"addedAt,omitempty"`
	UpdatedAt                              int               `json:"updatedAt,omitempty"`
	Thumb                                  string            `json:"thumb,omitempty"`
	Art                                    string            `json:"art,omitempty"`
	Theme                                  string            `json:"theme,omitempty"`
	Index                                  int               `json:"index,omitempty"`
	ChildCount                             int               `json:"childCount,omitempty"`
	SeasonCount                            int               `json:"seasonCount,omitempty"`
	LeafCount                              int               `json:"leafCount,omitempty"`
	ViewedLeafCount                        int               `json:"viewedLeafCount,omitempty"`
	EnableCreditsMarkerGeneration          int               `json:"enableCreditsMarkerGeneration,omitempty"`
	EpisodeSort                            int               `json:"episodeSort,omitempty"`
	FlattenSeasons                         int               `json:"flattenSeasons,omitempty"`
	ShowOrdering                           string            `json:"showOrdering,omitempty"`
	LanguageOverride                       string            `json:"languageOverride,omitempty"`
	UseOriginalTitle                       int               `json:"useOriginalTitle,omitempty"`
	AudioLanguage                          string            `json:"audioLanguage,omitempty"`
	SubtitleLanguage                       string            `json:"subtitleLanguage,omitempty"`
	SubtitleMode                           int               `json:"subtitleMode,omitempty"`
	AutoDeletionItemPolicyUnwatchedLibrary int               `json:"autoDeletionItemPolicyUnwatchedLibrary,omitempty"`
	AutoDeletionItemPolicyWatchedLibrary   int               `json:"autoDeletionItemPolicyWatchedLibrary,omitempty"`
	Slug                                   string            `json:"slug,omitempty"`
	SourceURI                              string            `json:"sourceURI,omitempty"`
	Label                                  []Label           `json:"Label,omitempty"`
	Genre                                  []Genre           `json:"Genre,omitempty"`
	Director                               []Director        `json:"Director,omitempty"`
	Writer                                 []Writer          `json:"Writer,omitempty"`
	Producer                               []Producer        `json:"Producer,omitempty"`
	Role                                   []Role            `json:"Role,omitempty"`
	Country                                []Country         `json:"Country,omitempty"`
	Collection                             []Collection      `json:"Collection,omitempty"`
	Guid                                   FlexibleGuid      `json:"Guid,omitempty"`
	Media                                  []Media           `json:"Media,omitempty"`
	Location                               []Location        `json:"Location,omitempty"`
}

// MediaItem interface implementation for TVShow
func (t TVShow) GetRatingKey() string { return t.RatingKey.String() }
func (t TVShow) GetTitle() string     { return t.Title }
func (t TVShow) GetYear() int         { return t.Year }
func (t TVShow) GetGuid() []Guid      { return []Guid(t.Guid) }
func (t TVShow) GetMedia() []Media    { return t.Media }
func (t TVShow) GetLabel() []Label    { return t.Label }
func (t TVShow) GetGenre() []Genre    { return t.Genre }

// Label represents a Plex label
type Label struct {
	Tag string `json:"tag"`
}

// Genre represents a Plex genre
type Genre struct {
	Tag string `json:"tag"`
}

// Guid represents a Plex GUID
type Guid struct {
	ID string `json:"id"`
}

// Media represents Plex media information
type Media struct {
	Part []Part `json:"Part,omitempty"`
}

// Part represents a media part with file information
type Part struct {
	File string `json:"file,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// FlexibleGuid handles both string and array formats from Plex API
type FlexibleGuid []Guid

func (fg *FlexibleGuid) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as array first
	var guidArray []Guid
	if err := json.Unmarshal(data, &guidArray); err == nil {
		*fg = FlexibleGuid(guidArray)
		return nil
	}

	// If that fails, try as single string
	var guidString string
	if err := json.Unmarshal(data, &guidString); err == nil {
		*fg = FlexibleGuid([]Guid{{ID: guidString}})
		return nil
	}

	// If both fail, try as single Guid object
	var singleGuid Guid
	if err := json.Unmarshal(data, &singleGuid); err == nil {
		*fg = FlexibleGuid([]Guid{singleGuid})
		return nil
	}

	return fmt.Errorf("cannot unmarshal Guid field")
}

// FlexibleRatingKey can handle both string and integer rating key values
type FlexibleRatingKey struct {
	Value string
}

// UnmarshalJSON implements custom JSON unmarshaling for FlexibleRatingKey
func (frk *FlexibleRatingKey) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string first
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		frk.Value = stringValue
		return nil
	}

	// If that fails, try to unmarshal as an integer and convert to string
	var intValue int
	if err := json.Unmarshal(data, &intValue); err == nil {
		frk.Value = fmt.Sprintf("%d", intValue)
		return nil
	}

	// If both fail, return error
	return fmt.Errorf("cannot unmarshal %s into FlexibleRatingKey", string(data))
}

// MarshalJSON implements custom JSON marshaling for FlexibleRatingKey
func (frk FlexibleRatingKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(frk.Value)
}

// String returns the string representation of the rating key
func (frk FlexibleRatingKey) String() string {
	return frk.Value
}

// FlexibleRating can handle both single rating values and arrays of ratings
type FlexibleRating struct {
	Value float64
}

// UnmarshalJSON implements custom JSON unmarshaling for FlexibleRating
func (fr *FlexibleRating) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a single float64 first
	var singleValue float64
	if err := json.Unmarshal(data, &singleValue); err == nil {
		fr.Value = singleValue
		return nil
	}

	// If that fails, try to unmarshal as an array and take the first value
	var arrayValue []float64
	if err := json.Unmarshal(data, &arrayValue); err == nil {
		if len(arrayValue) > 0 {
			fr.Value = arrayValue[0]
		}
		return nil
	}

	// If both fail, set to 0
	fr.Value = 0
	return nil
}

// MarshalJSON implements custom JSON marshaling for FlexibleRating
func (fr FlexibleRating) MarshalJSON() ([]byte, error) {
	return json.Marshal(fr.Value)
}

// MediaContainer holds metadata for movies or TV shows
type MediaContainer struct {
	Size     int     `json:"size"`
	Metadata []Movie `json:"Metadata"`
}

// TVShowContainer holds metadata for TV shows
type TVShowContainer struct {
	Size     int      `json:"size"`
	Metadata []TVShow `json:"Metadata"`
}

// PlexResponse represents a standard Plex API response for movies
type PlexResponse struct {
	MediaContainer MediaContainer `json:"MediaContainer"`
}

// TVShowResponse represents a Plex API response for TV shows
type TVShowResponse struct {
	MediaContainer TVShowContainer `json:"MediaContainer"`
}

// Episode represents a Plex episode
type Episode struct {
	RatingKey             FlexibleRatingKey `json:"ratingKey"`
	Title                 string            `json:"title"`
	TitleSort             string            `json:"titleSort,omitempty"`
	Summary               string            `json:"summary,omitempty"`
	Index                 int               `json:"index"`       // Episode number
	ParentIndex           int               `json:"parentIndex"` // Season number
	Year                  int               `json:"year,omitempty"`
	Duration              int               `json:"duration,omitempty"`
	ContentRating         string            `json:"contentRating,omitempty"`
	Rating                FlexibleRating    `json:"rating,omitempty"`
	AudienceRating        FlexibleRating    `json:"audienceRating,omitempty"`
	UserRating            FlexibleRating    `json:"userRating,omitempty"`
	OriginallyAvailableAt string            `json:"originallyAvailableAt,omitempty"`
	AddedAt               int               `json:"addedAt,omitempty"`
	UpdatedAt             int               `json:"updatedAt,omitempty"`
	Thumb                 string            `json:"thumb,omitempty"`
	Art                   string            `json:"art,omitempty"`
	ChapterSource         string            `json:"chapterSource,omitempty"`
	GrandparentTitle      string            `json:"grandparentTitle,omitempty"` // Show title
	GrandparentThumb      string            `json:"grandparentThumb,omitempty"`
	GrandparentArt        string            `json:"grandparentArt,omitempty"`
	GrandparentKey        string            `json:"grandparentKey,omitempty"`
	GrandparentRatingKey  FlexibleRatingKey `json:"grandparentRatingKey,omitempty"`
	GrandparentGuid       string            `json:"grandparentGuid,omitempty"`
	GrandparentSlug       string            `json:"grandparentSlug,omitempty"`
	GrandparentTheme      string            `json:"grandparentTheme,omitempty"`
	ParentTitle           string            `json:"parentTitle,omitempty"` // Season title
	ParentThumb           string            `json:"parentThumb,omitempty"`
	ParentKey             string            `json:"parentKey,omitempty"`
	ParentRatingKey       FlexibleRatingKey `json:"parentRatingKey,omitempty"`
	ParentGuid            string            `json:"parentGuid,omitempty"`
	ParentYear            int               `json:"parentYear,omitempty"`
	SkipParent            bool              `json:"skipParent,omitempty"`
	SourceURI             string            `json:"sourceURI,omitempty"`
	Label                 []Label           `json:"Label,omitempty"`
	Genre                 []Genre           `json:"Genre,omitempty"`
	Director              []Director        `json:"Director,omitempty"`
	Writer                []Writer          `json:"Writer,omitempty"`
	Producer              []Producer        `json:"Producer,omitempty"`
	Role                  []Role            `json:"Role,omitempty"`
	Collection            []Collection      `json:"Collection,omitempty"`
	Guid                  FlexibleGuid      `json:"Guid,omitempty"`
	Media                 []Media           `json:"Media,omitempty"`
}

// EpisodeContainer holds metadata for episodes
type EpisodeContainer struct {
	Size     int       `json:"size"`
	Metadata []Episode `json:"Metadata"`
}

// EpisodeResponse represents a Plex API response for episodes
type EpisodeResponse struct {
	MediaContainer EpisodeContainer `json:"MediaContainer"`
}

// WatchedState represents the watched state of a media item
type WatchedState struct {
	Watched      bool `json:"watched"`
	ViewCount    int  `json:"viewCount"`
	ViewOffset   int  `json:"viewOffset"`
	LastViewedAt int  `json:"lastViewedAt"`
}

// Activity represents a Plex server activity (like library scanning)
type Activity struct {
	UUID        string           `xml:"uuid,attr" json:"uuid"`
	Type        string           `xml:"type,attr" json:"type"`
	Cancellable int              `xml:"cancellable,attr" json:"cancellable"`
	UserID      int              `xml:"userID,attr" json:"userID"`
	Title       string           `xml:"title,attr" json:"title"`
	Subtitle    string           `xml:"subtitle,attr" json:"subtitle"`
	Progress    int              `xml:"progress,attr" json:"progress"`
	Context     *ActivityContext `xml:"Context" json:"context,omitempty"`
}

// ActivityContext provides additional context for activities
type ActivityContext struct {
	LibrarySectionID string `xml:"librarySectionID,attr" json:"librarySectionID"`
}

// ActivitiesResponse represents the response from /activities endpoint
type ActivitiesResponse struct {
	XMLName    xml.Name   `xml:"MediaContainer"`
	Size       int        `xml:"size,attr" json:"size"`
	Activities []Activity `xml:"Activity" json:"activities"`
}

// Director represents a Plex director
type Director struct {
	Tag string `json:"tag"`
}

// Writer represents a Plex writer
type Writer struct {
	Tag string `json:"tag"`
}

// Producer represents a Plex producer
type Producer struct {
	Tag string `json:"tag"`
}

// Role represents a Plex actor/role
type Role struct {
	Tag   string `json:"tag"`
	Role  string `json:"role,omitempty"`
	Thumb string `json:"thumb,omitempty"`
}

// Country represents a Plex country
type Country struct {
	Tag string `json:"tag"`
}

// Collection represents a Plex collection
type Collection struct {
	Tag string `json:"tag"`
}

// Location represents a Plex location
type Location struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}
