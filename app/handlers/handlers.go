package handlers

import (
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	validator *validator.Validate
}

func NewHandler() *Handler {
	return &Handler{
		validator: validator.New(),
	}
}

func (h *Handler) Home(c echo.Context, user *models.User) error {
	var posts []models.Post
	if err := models.DB.Where("published = ?", true).Order("created_at desc").Limit(5).Find(&posts).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch posts")
	}

	var accessiblePosts []models.Post
	for _, post := range posts {
		if post.CanAccess(user) {
			accessiblePosts = append(accessiblePosts, post)
		}
	}

	return render(c, templates.Layout("Home", templates.PostsList(accessiblePosts, user), user))
}

func (h *Handler) PostsList(c echo.Context, user *models.User) error {
	var posts []models.Post
	if err := models.DB.Where("published = ?", true).Order("created_at desc").Find(&posts).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch posts")
	}

	var accessiblePosts []models.Post
	for _, post := range posts {
		if post.CanAccess(user) {
			accessiblePosts = append(accessiblePosts, post)
		}
	}

	return render(c, templates.Layout("Posts", templates.PostsList(accessiblePosts, user), user))
}

func (h *Handler) PostView(c echo.Context, user *models.User) error {
	slug := c.Param("slug")

	var post models.Post
	if err := models.DB.Where("slug = ? AND published = ?", slug, true).First(&post).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Post not found")
	}

	if !post.CanAccess(user) {
		if user == nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "Login required to view this post")
		}
		return echo.NewHTTPError(http.StatusForbidden, "Access denied")
	}

	return render(c, templates.Layout(post.Title, templates.PostView(post), user))
}

func (h *Handler) AdminPostsList(c echo.Context, user *models.User) error {
	var posts []models.Post
	if err := models.DB.Order("created_at desc").Find(&posts).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch posts")
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		return render(c, templates.AdminPostsList(posts))
	}

	return render(c, templates.Layout("Admin - Posts", templates.AdminPostsList(posts), user))
}

func (h *Handler) AdminPostNew(c echo.Context, user *models.User) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		return render(c, templates.PostCreatePage())
	}

	return render(c, templates.AdminLayout("Create New Post", templates.PostCreatePage(), user))
}

func (h *Handler) AdminPostEdit(c echo.Context, user *models.User) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid post ID")
	}

	var post models.Post
	if err := models.DB.First(&post, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Post not found")
	}

	if c.Request().Header.Get("HX-Request") == "true" {
		return render(c, templates.PostEditPage(&post))
	}

	return render(c, templates.AdminLayout("Edit Post", templates.PostEditPage(&post), user))
}

func (h *Handler) AdminPostCreate(c echo.Context, user *models.User) error {
	visibility := c.FormValue("visibility")
	if visibility == "" {
		visibility = models.VisibilityPublic
	}

	post := &models.Post{
		Title:      c.FormValue("title"),
		Slug:       c.FormValue("slug"),
		Content:    c.FormValue("content"),
		Published:  c.FormValue("published") == "on",
		Visibility: visibility,
	}

	if err := h.validator.Struct(post); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Validation failed")
	}

	if err := models.DB.Create(post).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return echo.NewHTTPError(http.StatusConflict, "Post with this slug already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create post")
	}

	var posts []models.Post
	models.DB.Order("created_at desc").Find(&posts)
	return render(c, templates.AdminPostsList(posts))
}

func (h *Handler) AdminPostUpdate(c echo.Context, user *models.User) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid post ID")
	}

	var post models.Post
	if err := models.DB.First(&post, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Post not found")
	}

	visibility := c.FormValue("visibility")
	if visibility == "" {
		visibility = models.VisibilityPublic
	}

	post.Title = c.FormValue("title")
	post.Slug = c.FormValue("slug")
	post.Content = c.FormValue("content")
	post.Published = c.FormValue("published") == "on"
	post.Visibility = visibility

	if err := h.validator.Struct(post); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Validation failed")
	}

	if err := models.DB.Save(&post).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return echo.NewHTTPError(http.StatusConflict, "Post with this slug already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update post")
	}

	var posts []models.Post
	models.DB.Order("created_at desc").Find(&posts)
	return render(c, templates.AdminPostsList(posts))
}

func (h *Handler) AdminPostDelete(c echo.Context, user *models.User) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid post ID")
	}

	if err := models.DB.Delete(&models.Post{}, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete post")
	}

	return c.NoContent(http.StatusOK)
}

func render(c echo.Context, component templ.Component) error {
	return component.Render(c.Request().Context(), c.Response().Writer)
}
