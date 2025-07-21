package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"mini-blog/app/models"
)

var tmdbCallCounter int64

type TMDBService struct {
	BearerToken string
	BaseURL     string
	client      *http.Client
}

func NewTMDBService(bearerToken string) *TMDBService {
	return &TMDBService{
		BearerToken: bearerToken,
		BaseURL:     "https://api.themoviedb.org/3",
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Consolidated HTTP request method to eliminate duplication
func (s *TMDBService) doRequest(url string, target interface{}) error {
	// Simple TMDB API call counter and logging
	count := atomic.AddInt64(&tmdbCallCounter, 1)
	fmt.Printf("🌐 TMDB API CALL #%d: %s\n", count, url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+s.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("TMDB request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found on TMDB")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TMDB error: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// SearchResult mirrors TMDB's search response structure
type SearchResult struct {
	ID           int     `json:"id"`
	Title        string  `json:"title,omitempty"` // Movies
	Name         string  `json:"name,omitempty"`  // TV/Anime
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date,omitempty"`   // Movies
	FirstAirDate string  `json:"first_air_date,omitempty"` // TV/Anime
	Popularity   float64 `json:"popularity"`
	VoteCount    int     `json:"vote_count"`
	VoteAverage  float64 `json:"vote_average"`
}

// Search queries TMDB
func (s *TMDBService) Search(query string, mediaType string) ([]SearchResult, error) {
	endpoint := mediaType
	if mediaType == models.MediaTypeMovie {
		endpoint = "movie"
	} else if mediaType == models.MediaTypeTV {
		endpoint = "tv"
	} else {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}

	u := fmt.Sprintf("%s/search/%s?query=%s", s.BaseURL, endpoint, url.QueryEscape(query))

	var data struct {
		Results []SearchResult `json:"results"`
	}
	if err := s.doRequest(u, &data); err != nil {
		return nil, err
	}
	return data.Results, nil
}

// GetDetails fetches full details and maps to your Media model
func (s *TMDBService) GetDetails(tmdbID int, mediaType string) (*models.Media, error) {
	endpoint := mediaType
	if mediaType == models.MediaTypeMovie {
		endpoint = "movie"
	} else if mediaType == models.MediaTypeTV {
		endpoint = "tv"
	} else {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}

	u := fmt.Sprintf("%s/%s/%d", s.BaseURL, endpoint, tmdbID)

	var details struct {
		ID           int    `json:"id"`
		Title        string `json:"title,omitempty"`
		Name         string `json:"name,omitempty"`
		Overview     string `json:"overview"`
		PosterPath   string `json:"poster_path"`
		ReleaseDate  string `json:"release_date,omitempty"`
		FirstAirDate string `json:"first_air_date,omitempty"`
		Genres       []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"genres"`
		Popularity  float64 `json:"popularity"`
		VoteCount   int     `json:"vote_count"`
		VoteAverage float64 `json:"vote_average"`
	}

	if err := s.doRequest(u, &details); err != nil {
		return nil, err
	}

	// Map to Media - simplified mapping logic
	title := details.Title
	if title == "" {
		title = details.Name
	}
	releaseDateStr := details.ReleaseDate
	if releaseDateStr == "" {
		releaseDateStr = details.FirstAirDate
	}
	var releaseDate *time.Time
	if releaseDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", releaseDateStr); err == nil {
			releaseDate = &parsed
		}
	}

	genresJSON, _ := json.Marshal(details.Genres)

	return &models.Media{
		TMDBID:      details.ID,
		Type:        mediaType,
		Title:       title,
		Overview:    details.Overview,
		PosterPath:  details.PosterPath,
		ReleaseDate: releaseDate,
		Genres:      string(genresJSON),
		Popularity:  details.Popularity,
		VoteCount:   details.VoteCount,
		VoteAverage: details.VoteAverage,
	}, nil
}

// Season represents a TV show season
type Season struct {
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
	PosterPath   string `json:"poster_path"`
}

// Episode represents a TV show episode
type Episode struct {
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
	Runtime       int    `json:"runtime"`
	StillPath     string `json:"still_path"`
}

// GetSeasons fetches all seasons for a TV show
func (s *TMDBService) GetSeasons(tmdbID int) ([]Season, error) {
	u := fmt.Sprintf("%s/tv/%d", s.BaseURL, tmdbID)

	var details struct {
		Seasons []Season `json:"seasons"`
	}
	if err := s.doRequest(u, &details); err != nil {
		return nil, err
	}
	return details.Seasons, nil
}

// GetEpisodes fetches all episodes for a specific season
func (s *TMDBService) GetEpisodes(tmdbID int, seasonNumber int) ([]Episode, error) {
	u := fmt.Sprintf("%s/tv/%d/season/%d", s.BaseURL, tmdbID, seasonNumber)

	var seasonData struct {
		Episodes []Episode `json:"episodes"`
	}
	if err := s.doRequest(u, &seasonData); err != nil {
		return nil, err
	}
	return seasonData.Episodes, nil
}

// GetDetailedSeasons fetches seasons and maps to our local Season model
func (s *TMDBService) GetDetailedSeasons(tmdbID int) ([]models.Season, error) {
	u := fmt.Sprintf("%s/tv/%d", s.BaseURL, tmdbID)

	var details struct {
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			Name         string `json:"name"`
			Overview     string `json:"overview"`
			AirDate      string `json:"air_date"`
			EpisodeCount int    `json:"episode_count"`
			PosterPath   string `json:"poster_path"`
		} `json:"seasons"`
	}
	if err := s.doRequest(u, &details); err != nil {
		return nil, err
	}

	var seasons []models.Season
	for _, season := range details.Seasons {
		var airDate *time.Time
		if season.AirDate != "" {
			if parsed, err := time.Parse("2006-01-02", season.AirDate); err == nil {
				airDate = &parsed
			}
		}

		seasons = append(seasons, models.Season{
			TMDBID:       tmdbID,
			SeasonNumber: season.SeasonNumber,
			Name:         season.Name,
			Overview:     season.Overview,
			AirDate:      airDate,
			EpisodeCount: season.EpisodeCount,
			PosterPath:   season.PosterPath,
		})
	}
	return seasons, nil
}

// GetDetailedEpisodes fetches episodes and maps to our local Episode model
func (s *TMDBService) GetDetailedEpisodes(tmdbID int, seasonNumber int) ([]models.Episode, error) {
	u := fmt.Sprintf("%s/tv/%d/season/%d", s.BaseURL, tmdbID, seasonNumber)

	var seasonData struct {
		Episodes []struct {
			EpisodeNumber int     `json:"episode_number"`
			Name          string  `json:"name"`
			Overview      string  `json:"overview"`
			AirDate       string  `json:"air_date"`
			Runtime       int     `json:"runtime"`
			StillPath     string  `json:"still_path"`
			VoteAverage   float64 `json:"vote_average"`
			VoteCount     int     `json:"vote_count"`
		} `json:"episodes"`
	}
	if err := s.doRequest(u, &seasonData); err != nil {
		return nil, err
	}

	var episodes []models.Episode
	for _, episode := range seasonData.Episodes {
		var airDate *time.Time
		if episode.AirDate != "" {
			if parsed, err := time.Parse("2006-01-02", episode.AirDate); err == nil {
				airDate = &parsed
			}
		}

		episodes = append(episodes, models.Episode{
			TMDBID:        tmdbID,
			SeasonNumber:  seasonNumber,
			EpisodeNumber: episode.EpisodeNumber,
			Name:          episode.Name,
			Overview:      episode.Overview,
			AirDate:       airDate,
			Runtime:       episode.Runtime,
			StillPath:     episode.StillPath,
			VoteAverage:   episode.VoteAverage,
			VoteCount:     episode.VoteCount,
		})
	}
	return episodes, nil
}

// GetCallCount returns the current number of TMDB API calls made
func GetTMDBCallCount() int64 {
	return atomic.LoadInt64(&tmdbCallCounter)
}
