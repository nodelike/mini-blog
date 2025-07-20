package main

import (
	"log"
	"mini-blog/app/config"
	"mini-blog/app/handlers"
	"mini-blog/app/models"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := config.Load()

	models.ConnectDB(cfg)
	models.Migrate()
	models.CreateInitialAdmin(cfg)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Static("/static", "static")

	h := handlers.NewBaseHandler(cfg)

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
	admin.GET("/dashboard", h.AdminDashboard)
	admin.POST("/users/:id/role", h.AdminUpdateUserRole)

	// Admin post routes
	admin.GET("/posts/new", h.AdminPostNew)
	admin.GET("/posts/:id/edit", h.AdminPostEdit)
	admin.POST("/posts", h.AdminPostCreate)
	admin.PUT("/posts/:id", h.AdminPostUpdate)
	admin.DELETE("/posts/:id", h.AdminPostDelete)

	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Fatal(e.Start(":" + cfg.Server.Port))
}
