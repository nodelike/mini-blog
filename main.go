package main

import (
	"log"
	"mini-blog/app/config"
	"mini-blog/app/handlers"
	"mini-blog/app/models"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()

	// Initialize database
	models.ConnectDB(cfg)
	models.RunMigrations()
	models.CreateInitialAdmin(cfg)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Static("/static", "static")

	h := handlers.NewBaseHandler(cfg)

	// Health check route (no database dependency)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Public routes
	public := e.Group("")
	public.GET("/", h.Home)
	public.GET("/posts", h.Posts)
	public.GET("/posts/:slug", h.PostView)

	// Auth routes
	auth := e.Group("")
	auth.GET("/signup", h.SignupPage)
	auth.POST("/signup", h.Signup)
	auth.GET("/login", h.LoginPage)
	auth.POST("/login", h.Login)
	auth.POST("/verify-otp", h.VerifyOTP)
	auth.POST("/resend-otp", h.ResendOTP)
	auth.GET("/logout", h.Logout)

	// Admin routes
	admin := e.Group("/admin", h.RequireAdmin)
	{
		admin.GET("/dashboard", h.AdminDashboard)
		admin.POST("/users/:id/role", h.AdminUpdateUserRole)

		// Posts management
		admin.GET("/posts/new", h.AdminPostNew)
		admin.GET("/posts/:id/edit", h.AdminPostEdit)
		admin.POST("/posts", h.AdminPostCreate)
		admin.PUT("/posts/:id", h.AdminPostUpdate)
		admin.DELETE("/posts/:id", h.AdminPostDelete)
	}

	// Media Tracker routes
	tv := e.Group("/tv")
	{
		// Public routes
		tv.GET("", h.MediaList)
		tv.GET("/filter", h.MediaFilter)
		tv.GET("/search", h.MediaSearch)
		tv.GET("/modal/:id", h.MediaModal)
		tv.GET("/:tmdbId/episodes/:season", h.MediaEpisodes)

		// Admin-only routes
		admin := tv.Group("", h.RequireAdmin)
		{
			admin.POST("/add", h.MediaAdd)
			admin.PUT("/:id", h.MediaUpdate)
			admin.POST("/update/:tmdbId", h.MediaUpdateByTMDB)
			admin.DELETE("/:id", h.MediaDelete)
			admin.POST("/episodes/toggle/:tmdbId/:season/:episode", h.MarkEpisodeWatched)
			admin.POST("/mark-season/:tmdbId/:season", h.MarkSeasonWatched)
			admin.POST("/mark-show/:tmdbId", h.MarkShowWatched)
			admin.POST("/status/:tmdbId", h.MediaStatusUpdate)
			admin.POST("/toggle-anime/:tmdbId", h.MediaToggleAnime)
			admin.DELETE("/remove/:tmdbId", h.MediaRemove)
		}
	}

	// Start background sync
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			h.BackgroundSync()
		}
	}()

	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Fatal(e.Start(":" + cfg.Server.Port))
}
