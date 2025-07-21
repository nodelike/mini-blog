package handlers

import (
	"fmt"
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func (h *BaseHandler) MediaFilter(c echo.Context) error {
	user := h.GetCurrentUser(c)
	filters := c.QueryParams()["filters"]

	// Remove "all" filter and use unified function
	if len(filters) == 1 && filters[0] == "all" {
		filters = nil
	}

	media := h.getMediaSorted(filters, "")
	return h.render(c, templates.MediaGrid(media, user))
}

func (h *BaseHandler) MediaList(c echo.Context) error {
	user := h.GetCurrentUser(c)
	media := h.getMediaSorted(nil, "")

	if h.isHTMXRequest(c) {
		return h.render(c, templates.MediaGrid(media, user))
	}
	return h.render(c, templates.Layout("TV", templates.MediaTracker(media, user), c.Request().URL.Path, user))
}

func (h *BaseHandler) MediaSearch(c echo.Context) error {
	user := h.GetCurrentUser(c)
	query := strings.TrimSpace(c.QueryParam("query"))
	tmdbMode := c.QueryParam("tmdb_mode") == "true"
	mediaType := c.QueryParam("type")

	if query == "" {
		return h.render(c, templates.MediaGrid([]models.Media{}, user))
	}

	if tmdbMode {
		// TMDB search with specific type
		if user == nil || !user.IsAdmin() {
			return echo.NewHTTPError(http.StatusForbidden, "Admin access required for TMDB search")
		}
		if mediaType == "" {
			mediaType = "tv" // Default to TV if not specified
		}

		results, err := h.tmdbService.Search(query, mediaType)
		if err != nil {
			return h.render(c, templates.ErrorMessage("Failed to search TMDB"))
		}

		// Enrich with library status
		var enrichedResults []templates.EnrichedSearchResult
		for _, result := range results {
			var localMedia models.Media
			inLibrary := models.DB.Where("tmdb_id = ?", result.ID).First(&localMedia).Error == nil

			enrichedResults = append(enrichedResults, templates.EnrichedSearchResult{
				SearchResult: result,
				InLibrary:    inLibrary,
				LocalMedia:   localMedia,
			})
		}

		searchResults := templates.SearchResults{
			Results:   enrichedResults,
			MediaType: mediaType,
		}
		return h.render(c, templates.MediaGrid(searchResults, user))
	} else {
		// Library search (all types) with last watched sorting
		media := h.getMediaSorted(nil, query)
		return h.render(c, templates.MediaGrid(media, user))
	}
}

func (h *BaseHandler) MediaAdd(c echo.Context) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	tmdbID, mediaType, valid := h.parseMediaParams(c)
	status := c.FormValue("status")
	if status == "" {
		status = "planned" // Default to planned
	}

	if !valid || !models.IsValidStatus(status) {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid input")
	}

	// Check if media already exists
	var existing models.Media
	if models.DB.Where("tmdb_id = ?", tmdbID).First(&existing).Error == nil {
		return h.renderError(c, "Already tracking")
	}

	// Fetch from TMDB
	fetchedMedia, err := h.tmdbService.GetDetails(tmdbID, mediaType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch media")
	}

	// Set tracking fields
	fetchedMedia.Status = status
	fetchedMedia.AddedAt = time.Now()
	fetchedMedia.IsAnime = c.FormValue("is_anime") == "true"

	// Get total episodes for TV shows and store all episode data
	if mediaType == "tv" {
		if detailedSeasons, err := h.tmdbService.GetDetailedSeasons(tmdbID); err == nil {
			totalEpisodes := 0
			for _, season := range detailedSeasons {
				if season.SeasonNumber > 0 { // Exclude season 0 (specials)
					totalEpisodes += season.EpisodeCount

					// Store season
					var existingSeason models.Season
					if models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, season.SeasonNumber).First(&existingSeason).Error != nil {
						models.DB.Create(&season)
					}

					// Store all episodes for this season
					if detailedEpisodes, err := h.tmdbService.GetDetailedEpisodes(tmdbID, season.SeasonNumber); err == nil {
						for _, episode := range detailedEpisodes {
							var existingEpisode models.Episode
							if models.DB.Where("tmdb_id = ? AND season_number = ? AND episode_number = ?",
								tmdbID, season.SeasonNumber, episode.EpisodeNumber).First(&existingEpisode).Error != nil {

								// If adding as completed, mark aired episodes as watched
								if status == "completed" && (episode.AirDate == nil || episode.AirDate.Before(time.Now())) {
									episode.Watched = true
									now := time.Now()
									episode.WatchedAt = &now
								}

								models.DB.Create(&episode)
							}
						}
					}
				}
			}
			fetchedMedia.TotalEpisodes = totalEpisodes

			// Set progress if completed (count only aired episodes)
			if status == "completed" {
				var airedWatchedCount int64
				models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ? AND air_date <= ?", tmdbID, true, time.Now()).Count(&airedWatchedCount)
				fetchedMedia.Progress = int(airedWatchedCount)
			}
		}
	}

	if err := models.DB.Create(fetchedMedia).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to add to tracker")
	}

	return h.htmxRedirect(c, "/tv")
}

func (h *BaseHandler) MediaUpdate(c echo.Context) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	id, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	var media models.Media
	if err := models.DB.First(&media, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Media not found")
	}

	if status := h.trimFormValue(c, "status"); status != "" && models.IsValidStatus(status) {
		media.Status = status
	}

	if progressStr := h.trimFormValue(c, "progress"); progressStr != "" {
		if progress, err := strconv.Atoi(progressStr); err == nil {
			media.Progress = progress
		}
	}

	if ratingStr := h.trimFormValue(c, "rating"); ratingStr != "" {
		if rating, err := strconv.ParseFloat(ratingStr, 64); err == nil && rating >= 0 && rating <= 10 {
			media.Rating = rating
		}
	}

	media.Notes = h.trimFormValue(c, "notes")

	if err := models.DB.Save(&media).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update media")
	}

	return h.htmxRedirect(c, "/tv")
}

func (h *BaseHandler) MediaDelete(c echo.Context) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	id, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	if err := models.DB.Delete(&models.Media{}, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete media")
	}

	return c.NoContent(http.StatusOK)
}

func (h *BaseHandler) MediaModal(c echo.Context) error {
	user := h.GetCurrentUser(c)
	tmdbID, mediaType, valid := h.parseMediaParams(c)

	if !valid {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid input")
	}

	useLocal := models.DB.Where("tmdb_id = ?", tmdbID).First(&models.Media{}).Error == nil

	// Sync if stale (24h)
	if useLocal {
		var media models.Media
		models.DB.Where("tmdb_id = ?", tmdbID).First(&media)
		if media.LastSyncedAt == nil || media.LastSyncedAt.Before(time.Now().Add(-24*time.Hour)) {
			h.SyncMedia(tmdbID)
		}
	}

	media, seasons, episodes, allEpisodes, err := h.getMediaModalData(tmdbID, mediaType, useLocal)
	if err != nil {
		return h.render(c, templates.ErrorModal(err.Error()))
	}

	return h.render(c, templates.MediaDetailModal(media, seasons, episodes, allEpisodes, user))
}

func (h *BaseHandler) MediaEpisodes(c echo.Context) error {
	user := h.GetCurrentUser(c)
	tmdbID, _ := strconv.Atoi(c.Param("tmdbId"))
	season, _ := strconv.Atoi(c.Param("season"))

	if tmdbID == 0 || season == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid input")
	}

	// Check if show is in library first
	var media models.Media
	showInLibrary := models.DB.Where("tmdb_id = ?", tmdbID).First(&media).Error == nil

	var episodes []models.Episode
	var allEpisodes []models.Episode

	if showInLibrary {
		// Show is in library - get episodes from local database
		models.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, season).Order("episode_number ASC").Find(&episodes)
		models.DB.Where("tmdb_id = ?", tmdbID).Order("season_number ASC, episode_number ASC").Find(&allEpisodes)

		// Get seasons for response
		var seasons []models.Season
		models.DB.Where("tmdb_id = ?", tmdbID).Order("season_number ASC").Find(&seasons)

		return h.render(c, templates.SeasonResponse(media, seasons, episodes, allEpisodes, season, user, "update"))
	} else {
		// Show not in library - fetch from TMDB for preview
		if tmdbEpisodes, err := h.tmdbService.GetEpisodes(tmdbID, season); err == nil {
			for _, tmdbEpisode := range tmdbEpisodes {
				var airDate *time.Time
				if tmdbEpisode.AirDate != "" {
					if parsed, err := time.Parse("2006-01-02", tmdbEpisode.AirDate); err == nil {
						airDate = &parsed
					}
				}

				episodes = append(episodes, models.Episode{
					TMDBID:        tmdbID,
					SeasonNumber:  season,
					EpisodeNumber: tmdbEpisode.EpisodeNumber,
					Name:          tmdbEpisode.Name,
					Overview:      tmdbEpisode.Overview,
					AirDate:       airDate,
					StillPath:     tmdbEpisode.StillPath,
				})
			}
		}
		return h.render(c, templates.EpisodesListWithWatched(episodes, user))
	}
}

func (h *BaseHandler) MarkEpisodeWatched(c echo.Context) error {
	return h.markEpisodes(c, "episode")
}

func (h *BaseHandler) MarkSeasonWatched(c echo.Context) error {
	return h.markEpisodes(c, "season")
}

func (h *BaseHandler) MarkShowWatched(c echo.Context) error {
	return h.markEpisodes(c, "show")
}

func (h *BaseHandler) MediaUpdateByTMDB(c echo.Context) error {
	return h.updateMediaAndRefreshModal(c, func(media *models.Media) error {
		newStatus := h.trimFormValue(c, "status")
		if newStatus == "" || !models.IsValidStatus(newStatus) {
			return nil
		}

		media.Status = newStatus

		// If status is set to completed, mark all aired episodes as watched
		if newStatus == "completed" && media.Type == "tv" {
			now := time.Now()
			models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND air_date <= ?", media.TMDBID, time.Now()).
				Updates(models.Episode{Watched: true, WatchedAt: &now})

			var totalWatched int64
			models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ?", media.TMDBID, true).Count(&totalWatched)
			media.Progress = int(totalWatched)
		}

		return models.DB.Save(media).Error
	})
}

func (h *BaseHandler) MediaStatusUpdate(c echo.Context) error {
	return h.updateMediaAndRefreshModal(c, func(media *models.Media) error {
		newStatus := h.trimFormValue(c, "status")
		if newStatus == "" || !models.IsValidStatus(newStatus) {
			return nil // No update needed
		}

		media.Status = newStatus

		// Smart episode management for TV shows
		if media.Type == "tv" {
			if newStatus == "completed" {
				now := time.Now()
				models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND air_date <= ?", media.TMDBID, time.Now()).Updates(models.Episode{Watched: true, WatchedAt: &now})

				var totalWatched int64
				models.DB.Model(&models.Episode{}).Where("tmdb_id = ? AND watched = ?", media.TMDBID, true).Count(&totalWatched)
				media.Progress = int(totalWatched)
			} else if newStatus == "planned" {
				models.DB.Model(&models.Episode{}).Where("tmdb_id = ?", media.TMDBID).Updates(map[string]interface{}{"watched": false, "watched_at": nil})
				media.Progress = 0
			}
		}

		return models.DB.Save(media).Error
	})
}

func (h *BaseHandler) MediaToggleAnime(c echo.Context) error {
	return h.updateMediaAndRefreshModal(c, func(media *models.Media) error {
		media.IsAnime = !media.IsAnime
		return models.DB.Save(media).Error
	})
}

func (h *BaseHandler) MediaRemove(c echo.Context) error {
	_, err := h.requireAdmin(c)
	if err != nil {
		return err
	}

	tmdbIDParam := c.Param("tmdbId")
	if tmdbIDParam == "" {
		// Try alternative parameter name
		tmdbIDParam = c.Param("tmdbID")
	}

	tmdbID, err := strconv.Atoi(tmdbIDParam)
	if err != nil || tmdbID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid TMDB ID: %s", tmdbIDParam))
	}

	// Hard delete all related data (not soft delete)
	if err := models.DB.Unscoped().Where("tmdb_id = ?", tmdbID).Delete(&models.Episode{}).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete episodes")
	}

	if err := models.DB.Unscoped().Where("tmdb_id = ?", tmdbID).Delete(&models.Season{}).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete seasons")
	}

	if err := models.DB.Unscoped().Where("tmdb_id = ?", tmdbID).Delete(&models.Media{}).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete media")
	}

	// Close modal and redirect to refresh the page
	return c.HTML(http.StatusOK, `<script>
		closeModal();
		window.location.href = '/tv';
	</script>`)
}
