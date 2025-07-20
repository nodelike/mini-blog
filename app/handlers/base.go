package handlers

import (
	"mini-blog/app/config"
	"mini-blog/app/models"
	"mini-blog/app/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v4"
)

// BaseHandler consolidates all handler functionality
type BaseHandler struct {
	validator    *validator.Validate
	emailService *services.EmailService
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

func (h *BaseHandler) validateStruct(s interface{}) error {
	return h.validator.Struct(s)
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
