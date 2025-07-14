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

	// Static files
	e.Static("/static", "static")

	h := handlers.NewHandler()
	auth := handlers.NewAuthHandler(cfg)
	adminHandler := handlers.NewAdminHandler()

	e.GET("/", func(c echo.Context) error {
		return h.Home(c, auth.GetCurrentUser(c))
	})
	e.GET("/posts", func(c echo.Context) error {
		return h.PostsList(c, auth.GetCurrentUser(c))
	})
	e.GET("/posts/:slug", func(c echo.Context) error {
		return h.PostView(c, auth.GetCurrentUser(c))
	})

	e.GET("/signup", auth.SignupPage)
	e.POST("/signup", auth.Signup)
	e.GET("/login", auth.LoginPage)
	e.POST("/login", auth.Login)
	e.POST("/verify-otp", auth.VerifyOTP)
	e.POST("/resend-otp", auth.ResendOTP)
	e.GET("/logout", auth.Logout)

	admin := e.Group("/admin")
	admin.Use(auth.RequireAdmin)

	admin.GET("/dashboard", func(c echo.Context) error {
		return adminHandler.Dashboard(c, c.Get("user").(*models.User))
	})
	admin.GET("/users", func(c echo.Context) error {
		return adminHandler.UsersList(c, c.Get("user").(*models.User))
	})
	admin.POST("/users/:id/role", func(c echo.Context) error {
		return adminHandler.UpdateUserRole(c, c.Get("user").(*models.User))
	})
	admin.GET("/stats", func(c echo.Context) error {
		return adminHandler.Stats(c, c.Get("user").(*models.User))
	})

	admin.GET("/posts", func(c echo.Context) error {
		return h.AdminPostsList(c, c.Get("user").(*models.User))
	})
	admin.GET("/posts/new", func(c echo.Context) error {
		return h.AdminPostNew(c, c.Get("user").(*models.User))
	})
	admin.GET("/posts/:id/edit", func(c echo.Context) error {
		return h.AdminPostEdit(c, c.Get("user").(*models.User))
	})
	admin.POST("/posts", func(c echo.Context) error {
		return h.AdminPostCreate(c, c.Get("user").(*models.User))
	})
	admin.PUT("/posts/:id", func(c echo.Context) error {
		return h.AdminPostUpdate(c, c.Get("user").(*models.User))
	})
	admin.DELETE("/posts/:id", func(c echo.Context) error {
		return h.AdminPostDelete(c, c.Get("user").(*models.User))
	})

	log.Printf("Server starting on port %s", cfg.Server.Port)
	log.Fatal(e.Start(":" + cfg.Server.Port))
}
