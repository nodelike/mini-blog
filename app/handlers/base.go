package handlers

import (
	"mini-blog/app/config"
	"mini-blog/app/models"
	"mini-blog/app/services"
	"mini-blog/app/templates"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v4"
)

// BaseHandler consolidates all handler functionality
type BaseHandler struct {
	validator    *validator.Validate
	emailService *services.EmailService
	tmdbService  *services.TMDBService
	store        *sessions.CookieStore
	cfg          *config.Config
}

func NewBaseHandler(cfg *config.Config) *BaseHandler {
	store := sessions.NewCookieStore([]byte(cfg.Session.Key))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   cfg.Env == "production",
	}

	return &BaseHandler{
		validator:    validator.New(),
		emailService: services.NewEmailService(cfg),
		tmdbService:  services.NewTMDBService(cfg.TMDB.BearerToken),
		store:        store,
		cfg:          cfg,
	}
}

// Common utility methods
func (h *BaseHandler) render(c echo.Context, component templ.Component) error {
	return component.Render(c.Request().Context(), c.Response().Writer)
}

func (h *BaseHandler) GetCurrentUser(c echo.Context) *models.User {
	session, _ := h.store.Get(c.Request(), "auth-session")
	userID, ok := session.Values["user_id"].(uint)
	if !ok {
		return nil
	}

	var user models.User
	if err := models.DB.First(&user, userID).Error; err != nil {
		return nil
	}

	return &user
}

// Media-specific helper methods to reduce duplication
func (h *BaseHandler) requireAdmin(c echo.Context) (*models.User, error) {
	user := h.GetCurrentUser(c)
	if user == nil || !user.IsAdmin() {
		return nil, echo.NewHTTPError(http.StatusForbidden, "Admin access required")
	}
	return user, nil
}

func (h *BaseHandler) parseMediaParams(c echo.Context) (tmdbID int, mediaType string, valid bool) {
	tmdbID, _ = strconv.Atoi(c.Param("id"))
	if tmdbID == 0 {
		tmdbID, _ = strconv.Atoi(c.FormValue("tmdb_id"))
	}
	mediaType = c.QueryParam("type")
	if mediaType == "" {
		mediaType = c.FormValue("type")
	}
	valid = tmdbID > 0 && (mediaType == "movie" || mediaType == "tv")
	return
}

func (h *BaseHandler) parseEpisodeParams(c echo.Context) (tmdbID, season, episode int, valid bool) {
	tmdbID, _ = strconv.Atoi(c.Param("tmdbId"))
	season, _ = strconv.Atoi(c.Param("season"))
	episode, _ = strconv.Atoi(c.Param("episode"))
	valid = tmdbID > 0 && season > 0 && episode > 0
	return
}

func (h *BaseHandler) renderError(c echo.Context, message string) error {
	return h.render(c, templates.ErrorMessage(message))
}

func (h *BaseHandler) htmxRedirect(c echo.Context, url string) error {
	c.Response().Header().Set("HX-Redirect", url)
	return c.NoContent(http.StatusOK)
}

// getMediaModalData: Centralized modal data fetching (DRY for 6+ handlers)
func (h *BaseHandler) getMediaModalData(tmdbID int, mediaType string, useLocal bool) (*models.Media, []models.Season, []models.Episode, []models.Episode, error) {
	var media models.Media

	if useLocal {
		if err := models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error; err != nil {
			return nil, nil, nil, nil, err
		}
	} else {
		fetched, err := h.tmdbService.GetDetails(tmdbID, mediaType)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		media = *fetched
	}

	var seasons []models.Season
	var episodes, allEpisodes []models.Episode

	if mediaType == "tv" {
		if useLocal {
			// Get from local database
			models.DB.Where("tmdb_id = ? AND season_number > 0", tmdbID).Order("season_number ASC").Find(&seasons)
			models.DB.Where("tmdb_id = ?", tmdbID).Find(&allEpisodes)

			if len(seasons) > 0 {
				lastSeason := h.getLastWatchedSeason(allEpisodes)
				models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, lastSeason).Order("episode_number ASC").Find(&episodes)
			}
		} else {
			// Fetch from TMDB for preview
			if tmdbSeasons, err := h.tmdbService.GetSeasons(tmdbID); err == nil {
				for _, tmdbSeason := range tmdbSeasons {
					if tmdbSeason.SeasonNumber > 0 {
						seasons = append(seasons, models.Season{
							TMDBID:       tmdbID,
							SeasonNumber: tmdbSeason.SeasonNumber,
							Name:         tmdbSeason.Name,
							EpisodeCount: tmdbSeason.EpisodeCount,
							PosterPath:   tmdbSeason.PosterPath,
						})
					}
				}

				if len(seasons) > 0 {
					if detailedEpisodes, err := h.tmdbService.GetDetailedEpisodes(tmdbID, seasons[0].SeasonNumber); err == nil {
						episodes = detailedEpisodes
						allEpisodes = episodes
					}
				}
			}
		}
	}

	return &media, seasons, episodes, allEpisodes, nil
}

// getLastWatchedSeason: Helper for modal data fetching
func (h *BaseHandler) getLastWatchedSeason(episodes []models.Episode) int {
	lastSeason := 1
	var lastWatchedTime *time.Time

	for _, episode := range episodes {
		if episode.Watched && episode.WatchedAt != nil && episode.SeasonNumber > 0 {
			if lastWatchedTime == nil || episode.WatchedAt.After(*lastWatchedTime) {
				lastWatchedTime = episode.WatchedAt
				lastSeason = episode.SeasonNumber
			}
		}
	}
	return lastSeason
}

// Generic media status update with modal refresh (DRY for multiple handlers)
func (h *BaseHandler) updateMediaAndRefreshModal(c echo.Context, updateFn func(*models.Media) error) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	tmdbID, _ := strconv.Atoi(c.Param("tmdbId"))
	if tmdbID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid TMDB ID")
	}

	var media models.Media
	if err := models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Media not found")
	}

	if err := updateFn(&media); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	refreshedMedia, seasons, episodes, allEpisodes, err := h.getMediaModalData(tmdbID, media.Type, true)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to refresh modal")
	}

	return h.render(c, templates.MediaDetailModal(refreshedMedia, seasons, episodes, allEpisodes, h.GetCurrentUser(c)))
}

// Generic episode marking function (DRY for MarkEpisodeWatched, MarkSeasonWatched, MarkShowWatched)
func (h *BaseHandler) markEpisodes(c echo.Context, scope string) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	tmdbID, _ := strconv.Atoi(c.Param("tmdbId"))
	if tmdbID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid TMDB ID")
	}

	var episodes []models.Episode
	var whereClause string
	var whereArgs []interface{}

	// Build query based on scope
	switch scope {
	case "episode":
		seasonNumber, _ := strconv.Atoi(c.Param("season"))
		episodeNumber, _ := strconv.Atoi(c.Param("episode"))
		if seasonNumber == 0 || episodeNumber == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid input")
		}
		whereClause = "tmdb_id = ? AND season_number = ? AND episode_number = ? AND (air_date IS NULL OR air_date <= ?)"
		whereArgs = []interface{}{tmdbID, seasonNumber, episodeNumber, time.Now()}
	case "season":
		seasonNumber, _ := strconv.Atoi(c.Param("season"))
		if seasonNumber == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid season")
		}
		whereClause = "tmdb_id = ? AND season_number = ? AND air_date <= ?"
		whereArgs = []interface{}{tmdbID, seasonNumber, time.Now()}
	case "show":
		whereClause = "tmdb_id = ? AND air_date <= ?"
		whereArgs = []interface{}{tmdbID, time.Now()}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid scope")
	}

	// Get episodes to mark
	models.DB.Where(whereClause, whereArgs...).Find(&episodes)
	if len(episodes) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "No aired episodes found")
	}

	// Check current state and toggle appropriately
	watchedCount := 0
	for _, ep := range episodes {
		if ep.Watched {
			watchedCount++
		}
	}

	allWatched := watchedCount == len(episodes)
	now := time.Now()

	// Update episodes
	if allWatched {
		models.DB.Model(&models.Episode{}).Where(whereClause, whereArgs...).
			Updates(models.Episode{Watched: false, WatchedAt: nil})
	} else {
		models.DB.Model(&models.Episode{}).Where(whereClause, whereArgs...).
			Updates(models.Episode{Watched: true, WatchedAt: &now})
	}

	// Update media progress and status
	h.updateMediaProgress(tmdbID)

	// Return appropriate response based on scope
	switch scope {
	case "episode":
		var episode models.Episode
		models.DB.Where(whereClause, whereArgs...).First(&episode)
		return h.render(c, templates.UnifiedEpisodeRow(episode, h.GetCurrentUser(c)))
	case "season":
		return h.renderSeasonResponse(c, tmdbID, whereArgs[1].(int))
	case "show":
		return h.htmxRedirect(c, "/tv")
	}

	return c.NoContent(http.StatusOK)
}

// Helper to update media progress after episode changes
func (h *BaseHandler) updateMediaProgress(tmdbID int) {
	var media models.Media
	if models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error == nil {
		var totalWatched int64
		models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ?", tmdbID, true).Count(&totalWatched)
		media.Progress = int(totalWatched)

		if totalWatched == 0 {
			media.Status = "planned"
		} else if media.TotalEpisodes > 0 && int(totalWatched) >= media.TotalEpisodes {
			media.Status = "completed"
		} else {
			media.Status = "watching"
		}
		models.DB.Save(&media)
	}
}

// Helper to render season response
func (h *BaseHandler) renderSeasonResponse(c echo.Context, tmdbID, seasonNumber int) error {
	var episodes []models.Episode
	models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, seasonNumber).Order("episode_number ASC").Find(&episodes)

	var seasons []models.Season
	var allEpisodes []models.Episode
	var media models.Media
	models.DB.Where("tmdb_id = ? AND season_number > 0", tmdbID).Order("season_number ASC").Find(&seasons)
	models.DB.Where("tmdb_id = ?", tmdbID).Find(&allEpisodes)
	models.DB.Where("tmdb_id = ?", tmdbID).First(&media)

	return h.render(c, templates.SeasonUpdateResponse(media, seasons, episodes, allEpisodes, seasonNumber, h.GetCurrentUser(c)))
}

func (h *BaseHandler) setUserSession(c echo.Context, userID uint) error {
	session, _ := h.store.Get(c.Request(), "auth-session")
	session.Values["user_id"] = userID
	return session.Save(c.Request(), c.Response())
}

func (h *BaseHandler) clearUserSession(c echo.Context) error {
	session, _ := h.store.Get(c.Request(), "auth-session")
	session.Values["user_id"] = nil
	session.Options.MaxAge = -1
	return session.Save(c.Request(), c.Response())
}

// Common post filtering logic
func (h *BaseHandler) getAccessiblePosts(posts []models.Post, user *models.User) []models.Post {
	var accessible []models.Post
	for _, post := range posts {
		if post.CanAccess(user) {
			accessible = append(accessible, post)
		}
	}
	return accessible
}

// Middleware
func (h *BaseHandler) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := h.GetCurrentUser(c)
		if user == nil {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		c.Set("user", user)
		return next(c)
	}
}

func (h *BaseHandler) RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := h.GetCurrentUser(c)
		if user == nil {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		if !user.IsAdmin() {
			return echo.NewHTTPError(http.StatusForbidden, "Admin access required")
		}
		c.Set("user", user)
		return next(c)
	}
}

// Helper functions
func (h *BaseHandler) parseUintParam(c echo.Context, param string) (uint, error) {
	id, err := strconv.ParseUint(c.Param(param), 10, 32)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "Invalid "+param)
	}
	return uint(id), nil
}

func (h *BaseHandler) trimFormValue(c echo.Context, key string) string {
	return strings.TrimSpace(c.FormValue(key))
}

func (h *BaseHandler) isHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// SyncMedia updates a media item from TMDB (minimal implementation)
func (h *BaseHandler) SyncMedia(tmdbID int) error {
	var media models.Media
	if err := models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error; err != nil {
		return err
	}

	// Fetch fresh details
	freshMedia, err := h.tmdbService.GetDetails(tmdbID, media.Type)
	if err != nil {
		return err
	}

	// Update non-user fields
	media.Title = freshMedia.Title
	media.Overview = freshMedia.Overview
	media.PosterPath = freshMedia.PosterPath
	media.VoteCount = freshMedia.VoteCount
	media.VoteAverage = freshMedia.VoteAverage
	now := time.Now()
	media.LastSyncedAt = &now

	models.DB.Save(&media)

	// Sync episodes for TV shows
	if media.Type == "tv" {
		detailedSeasons, _ := h.tmdbService.GetDetailedSeasons(tmdbID)
		totalEpisodes := 0

		for _, season := range detailedSeasons {
			if season.SeasonNumber > 0 {
				totalEpisodes += season.EpisodeCount

				// Upsert season
				var existingSeason models.Season
				if models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, season.SeasonNumber).First(&existingSeason).Error != nil {
					models.DB.Create(&season)
				} else {
					existingSeason.Name = season.Name
					existingSeason.EpisodeCount = season.EpisodeCount
					models.DB.Save(&existingSeason)
				}

				// Sync episodes
				detailedEpisodes, _ := h.tmdbService.GetDetailedEpisodes(tmdbID, season.SeasonNumber)
				for _, episode := range detailedEpisodes {
					var existingEpisode models.Episode
					if models.DB.Where("tmdb_id = ? AND season_number = ? AND episode_number = ?",
						tmdbID, season.SeasonNumber, episode.EpisodeNumber).First(&existingEpisode).Error != nil {
						models.DB.Create(&episode)
					} else {
						existingEpisode.Name = episode.Name
						existingEpisode.Overview = episode.Overview
						existingEpisode.AirDate = episode.AirDate
						models.DB.Save(&existingEpisode)
					}
				}
			}
		}

		media.TotalEpisodes = totalEpisodes
		var watchedCount int64
		models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ?", tmdbID, true).Count(&watchedCount)
		media.Progress = int(watchedCount)
		models.DB.Save(&media)
	}

	return nil
}

// BackgroundSync syncs all active media (minimal background job)
func (h *BaseHandler) BackgroundSync() {
	var mediaItems []models.Media
	models.DB.Where("status IN ?", []string{"watching", "planned"}).Find(&mediaItems)

	for _, m := range mediaItems {
		if m.LastSyncedAt == nil || m.LastSyncedAt.Before(time.Now().Add(-48*time.Hour)) {
			h.SyncMedia(m.TMDBID)
			time.Sleep(500 * time.Millisecond) // Rate limit
		}
	}
}
