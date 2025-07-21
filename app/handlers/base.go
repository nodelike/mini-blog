package handlers

import (
	"fmt"
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
	"gorm.io/gorm"
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

func (h *BaseHandler) renderWithCardUpdate(c echo.Context, component templ.Component, media models.Media) error {
	c.Response().Header().Set("Content-Type", "text/html")
	c.Response().WriteHeader(http.StatusOK)

	// Render main content
	component.Render(c.Request().Context(), c.Response().Writer)

	// Update search card out-of-band
	c.Response().Writer.Write([]byte(fmt.Sprintf(`<div hx-swap-oob="true" id="tmdb-%d">`, media.TMDBID)))
	templates.UnifiedMediaCard(media, h.GetCurrentUser(c), false).Render(c.Request().Context(), c.Response().Writer)
	c.Response().Writer.Write([]byte(`</div>`))

	return nil
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

// getMediaModalData: Centralized modal data fetching
func (h *BaseHandler) getMediaModalData(tmdbID int, mediaType string, useLocal bool) (*models.Media, []models.Season, []models.Episode, []models.Episode, error) {
	media, err := h.getMediaData(tmdbID, mediaType, useLocal)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if mediaType != "tv" {
		return media, nil, nil, nil, nil
	}

	seasons, episodes, allEpisodes := h.getTVData(tmdbID, useLocal)
	return media, seasons, episodes, allEpisodes, nil
}

func (h *BaseHandler) getMediaData(tmdbID int, mediaType string, useLocal bool) (*models.Media, error) {
	if useLocal {
		var media models.Media
		err := models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error
		return &media, err
	}
	return h.tmdbService.GetDetails(tmdbID, mediaType)
}

func (h *BaseHandler) getTVData(tmdbID int, useLocal bool) ([]models.Season, []models.Episode, []models.Episode) {
	if useLocal {
		var seasons []models.Season
		var allEpisodes []models.Episode
		models.DB.Where("tmdb_id = ? AND season_number > 0", tmdbID).Order("season_number ASC").Find(&seasons)
		models.DB.Where("tmdb_id = ?", tmdbID).Find(&allEpisodes)

		var episodes []models.Episode
		if len(seasons) > 0 {
			lastSeason := h.getLastWatchedSeason(allEpisodes)
			models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, lastSeason).Order("episode_number ASC").Find(&episodes)
		}
		return seasons, episodes, allEpisodes
	}

	// TMDB preview data
	tmdbSeasons, err := h.tmdbService.GetSeasons(tmdbID)
	if err != nil {
		return nil, nil, nil
	}

	var seasons []models.Season
	for _, s := range tmdbSeasons {
		if s.SeasonNumber > 0 {
			seasons = append(seasons, models.Season{
				TMDBID: tmdbID, SeasonNumber: s.SeasonNumber,
				Name: s.Name, EpisodeCount: s.EpisodeCount, PosterPath: s.PosterPath,
			})
		}
	}

	var episodes []models.Episode
	if len(seasons) > 0 {
		if eps, err := h.tmdbService.GetDetailedEpisodes(tmdbID, seasons[0].SeasonNumber); err == nil {
			episodes = eps
		}
	}
	return seasons, episodes, episodes
}

// getMediaSorted: Unified media fetching with optional filters and search, sorted by last watched
func (h *BaseHandler) getMediaSorted(filters []string, searchTerm string) []models.Media {
	var media []models.Media
	var conditions []string
	var args []interface{}

	// Build filter conditions
	for _, filter := range filters {
		switch filter {
		case "tv":
			conditions = append(conditions, "(m.type = ? AND m.is_anime = ?)")
			args = append(args, "tv", false)
		case "movie":
			conditions = append(conditions, "(m.type = ? AND m.is_anime = ?)")
			args = append(args, "movie", false)
		case "anime-tv":
			conditions = append(conditions, "(m.type = ? AND m.is_anime = ?)")
			args = append(args, "tv", true)
		case "anime-movie":
			conditions = append(conditions, "(m.type = ? AND m.is_anime = ?)")
			args = append(args, "movie", true)
		}
	}

	// Add search condition
	if searchTerm != "" {
		conditions = append(conditions, "m.title ILIKE ?")
		args = append(args, "%"+searchTerm+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		// For filters use OR, for search combine with AND
		if searchTerm != "" && len(conditions) > 1 {
			filterClause := strings.Join(conditions[:len(conditions)-1], " OR ")
			whereClause = "WHERE (" + filterClause + ") AND " + conditions[len(conditions)-1]
		} else if searchTerm != "" {
			whereClause = "WHERE " + conditions[len(conditions)-1]
		} else {
			whereClause = "WHERE " + strings.Join(conditions, " OR ")
		}
	}

	models.DB.Raw(`
		SELECT m.* FROM media m
		LEFT JOIN (
			SELECT tmdb_id, MAX(watched_at) as last_episode_watched
			FROM episodes 
			WHERE watched = true
			GROUP BY tmdb_id
		) e ON m.tmdb_id = e.tmdb_id
		`+whereClause+`
		ORDER BY 
			CASE 
				WHEN m.type = 'tv' AND e.last_episode_watched IS NOT NULL THEN e.last_episode_watched
				ELSE m.updated_at
			END DESC NULLS LAST
	`, args...).Find(&media)

	return media
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

	whereClause, whereArgs := h.buildEpisodeQuery(scope, c, tmdbID)
	if whereClause == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid parameters")
	}

	freshDB := models.DB.Session(&gorm.Session{NewDB: true})
	var episodes []models.Episode
	freshDB.Where(whereClause, whereArgs...).Find(&episodes)
	if len(episodes) == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "No aired episodes found")
	}

	// Toggle based on current state
	allWatched := h.countWatched(episodes) == len(episodes)
	now := time.Now()

	txErr := freshDB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{"watched": !allWatched}
		if !allWatched {
			updates["watched_at"] = now
		} else {
			updates["watched_at"] = nil
		}
		return tx.Model(&models.Episode{}).Where(whereClause, whereArgs...).Updates(updates).Error
	})

	if txErr != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update episodes")
	}

	time.Sleep(10 * time.Millisecond)
	h.updateMediaProgress(tmdbID)

	return h.handleEpisodeResponse(c, scope, whereClause, whereArgs, tmdbID)
}

// Helper functions for episode operations
func (h *BaseHandler) buildEpisodeQuery(scope string, c echo.Context, tmdbID int) (string, []interface{}) {
	now := time.Now()
	switch scope {
	case "episode":
		season, _ := strconv.Atoi(c.Param("season"))
		episode, _ := strconv.Atoi(c.Param("episode"))
		if season == 0 || episode == 0 {
			return "", nil
		}
		return "tmdb_id = ? AND season_number = ? AND episode_number = ? AND (air_date IS NULL OR air_date <= ?)",
			[]interface{}{tmdbID, season, episode, now}
	case "season":
		season, _ := strconv.Atoi(c.Param("season"))
		if season == 0 {
			return "", nil
		}
		return "tmdb_id = ? AND season_number = ? AND air_date <= ?",
			[]interface{}{tmdbID, season, now}
	case "show":
		return "tmdb_id = ? AND air_date <= ?", []interface{}{tmdbID, now}
	}
	return "", nil
}

func (h *BaseHandler) countWatched(episodes []models.Episode) int {
	count := 0
	for _, ep := range episodes {
		if ep.Watched {
			count++
		}
	}
	return count
}

func (h *BaseHandler) handleEpisodeResponse(c echo.Context, scope, whereClause string, whereArgs []interface{}, tmdbID int) error {
	switch scope {
	case "episode":
		var episode models.Episode
		models.DB.Where(whereClause, whereArgs...).First(&episode)
		return h.render(c, templates.UnifiedEpisodeRow(episode, h.GetCurrentUser(c)))
	case "season":
		return h.renderSeasonResponse(c, tmdbID, whereArgs[1].(int), "toggle")
	case "show":
		return h.htmxRedirect(c, "/tv")
	}
	return c.NoContent(http.StatusOK)
}

// Helper to update media progress after episode changes
func (h *BaseHandler) updateMediaProgress(tmdbID int) {
	// Use fresh database session to ensure accurate counts
	freshDB := models.DB.Session(&gorm.Session{NewDB: true})

	var media models.Media
	if freshDB.Where("tmdb_id = ?", tmdbID).First(&media).Error == nil {
		var totalWatched int64
		var totalAired int64

		// Count watched episodes
		freshDB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ?", tmdbID, true).Count(&totalWatched)
		// Count aired episodes for proper completion calculation
		freshDB.Model(&models.Episode{}).Where("tmdb_id = ? AND air_date <= ?", tmdbID, time.Now()).Count(&totalAired)

		media.Progress = int(totalWatched)

		if totalWatched == 0 {
			media.Status = "planned"
		} else if totalAired > 0 && totalWatched >= totalAired {
			media.Status = "completed"
		} else {
			media.Status = "watching"
		}
		freshDB.Save(&media)
	}
}

// Unified season data fetcher and renderer
func (h *BaseHandler) renderSeasonResponse(c echo.Context, tmdbID, seasonNumber int, updateType string) error {
	h.updateMediaProgress(tmdbID)

	// Get fresh data
	freshDB := models.DB.Session(&gorm.Session{NewDB: true})
	episodes, seasons, allEpisodes, media := h.getSeasonData(freshDB, tmdbID, seasonNumber)

	return h.render(c, templates.SeasonResponse(media, seasons, episodes, allEpisodes, seasonNumber, h.GetCurrentUser(c), updateType))
}

// Consolidated data fetcher
func (h *BaseHandler) getSeasonData(db *gorm.DB, tmdbID, seasonNumber int) ([]models.Episode, []models.Season, []models.Episode, models.Media) {
	var episodes []models.Episode
	var seasons []models.Season
	var allEpisodes []models.Episode
	var media models.Media

	db.Where("tmdb_id = ? AND season_number = ?", tmdbID, seasonNumber).Order("episode_number ASC").Find(&episodes)
	db.Where("tmdb_id = ? AND season_number > 0", tmdbID).Order("season_number ASC").Find(&seasons)
	db.Where("tmdb_id = ?", tmdbID).Order("season_number ASC, episode_number ASC").Find(&allEpisodes)
	db.Where("tmdb_id = ?", tmdbID).First(&media)

	return episodes, seasons, allEpisodes, media
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
	media.InProduction = freshMedia.InProduction
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

// syncInProduction: Helper to sync production status from TMDB
func (h *BaseHandler) syncInProduction(media *models.Media) {
	if freshMedia, err := h.tmdbService.GetDetails(media.TMDBID, media.Type); err == nil {
		media.InProduction = freshMedia.InProduction
	}
}
