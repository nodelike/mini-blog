package handlers

import (
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
)

// Public Post handlers
func (h *BaseHandler) Home(c echo.Context) error {
	user := h.GetCurrentUser(c)

	var posts []models.Post
	query := models.DB.Where("published = ?", true).Order("created_at desc").Limit(5)

	if err := query.Find(&posts).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch posts")
	}

	accessible := h.getAccessiblePosts(posts, user)
	return h.render(c, templates.Layout("Home", templates.PostsList(accessible, "Latest Posts", false, "", true, user), c.Request().URL.Path, user))
}

func (h *BaseHandler) Posts(c echo.Context) error {
	user := h.GetCurrentUser(c)
	searchQuery := h.trimFormValue(c, "search")

	var posts []models.Post
	query := models.DB.Where("published = ?", true)

	if searchQuery != "" {
		searchTerm := "%" + searchQuery + "%"
		query = query.Where("title ILIKE ? OR content ILIKE ?", searchTerm, searchTerm)
	}

	query = query.Order("created_at desc")

	if err := query.Find(&posts).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch posts")
	}

	accessible := h.getAccessiblePosts(posts, user)

	// Return just the posts content for HTMX requests
	if h.isHTMXRequest(c) {
		return h.render(c, templates.PostsContent(accessible, false))
	}

	return h.render(c, templates.Layout("Posts", templates.PostsList(accessible, "Blog Posts", true, searchQuery, false, user), c.Request().URL.Path, user))
}

func (h *BaseHandler) PostView(c echo.Context) error {
	slug := c.Param("slug")
	user := h.GetCurrentUser(c)

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

	return h.render(c, templates.Layout(post.Title, templates.PostView(post), c.Request().URL.Path, user))
}

// Admin dashboard
func (h *BaseHandler) AdminDashboard(c echo.Context) error {
	user := c.Get("user").(*models.User)

	// Fetch users
	var users []models.User
	models.DB.Order("created_at desc").Find(&users)

	// Fetch posts
	var posts []models.Post
	models.DB.Order("created_at desc").Find(&posts)

	// Calculate stats
	stats := models.DashboardStats{}
	models.DB.Model(&models.User{}).Count(&stats.TotalUsers)
	models.DB.Model(&models.User{}).Where("role IN ?", []string{models.RolePremium, models.RoleAdmin}).Count(&stats.PremiumUsers)
	models.DB.Model(&models.Post{}).Count(&stats.TotalPosts)
	models.DB.Model(&models.Post{}).Where("published = ?", true).Count(&stats.PublishedPosts)

	if h.isHTMXRequest(c) {
		return h.render(c, templates.AdminDashboard(users, posts, stats))
	}
	return h.render(c, templates.Layout("Admin Dashboard", templates.AdminDashboard(users, posts, stats), c.Request().URL.Path, user))
}

// Admin user management
func (h *BaseHandler) AdminUpdateUserRole(c echo.Context) error {
	userID, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	newRole := c.FormValue("role")
	if !models.IsValidRole(newRole) {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid role")
	}

	var targetUser models.User
	if err := models.DB.First(&targetUser, userID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "User not found")
	}

	if err := models.DB.Model(&targetUser).Update("role", newRole).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user role")
	}

	if err := models.DB.First(&targetUser, userID).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to reload user")
	}

	return h.render(c, templates.AdminUserRow(targetUser))
}

// Admin post management
func (h *BaseHandler) AdminPostNew(c echo.Context) error {
	user := c.Get("user").(*models.User)
	if h.isHTMXRequest(c) {
		return h.render(c, templates.PostCreatePage())
	}
	return h.render(c, templates.Layout("Create New Post", templates.PostCreatePage(), c.Request().URL.Path, user))
}

func (h *BaseHandler) AdminPostEdit(c echo.Context) error {
	id, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	var post models.Post
	if err := models.DB.First(&post, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Post not found")
	}

	user := c.Get("user").(*models.User)
	if h.isHTMXRequest(c) {
		return h.render(c, templates.PostEditPage(&post))
	}
	return h.render(c, templates.Layout("Edit Post", templates.PostEditPage(&post), c.Request().URL.Path, user))
}

func (h *BaseHandler) AdminPostCreate(c echo.Context) error {
	title, content := h.trimFormValue(c, "title"), h.trimFormValue(c, "content")
	if title == "" || content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Title and content are required")
	}

	slug := h.trimFormValue(c, "slug")
	if slug == "" {
		slug = h.generateSlug(title)
	}

	visibility := c.FormValue("visibility")
	if !models.IsValidVisibility(visibility) {
		visibility = models.VisibilityPublic
	}

	if err := models.DB.Create(&models.Post{
		Title: title, Slug: slug, Content: content,
		Visibility: visibility, Published: c.FormValue("published") == "on",
	}).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create post")
	}

	c.Response().Header().Set("HX-Redirect", "/admin/dashboard")
	return c.NoContent(http.StatusOK)
}

func (h *BaseHandler) AdminPostUpdate(c echo.Context) error {
	id, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	var post models.Post
	if err := models.DB.First(&post, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Post not found")
	}

	post.Title, post.Content = h.trimFormValue(c, "title"), h.trimFormValue(c, "content")
	if post.Title == "" || post.Content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Title and content are required")
	}

	post.Slug = h.trimFormValue(c, "slug")
	if post.Slug == "" {
		post.Slug = h.generateSlug(post.Title)
	}

	post.Visibility = c.FormValue("visibility")
	if !models.IsValidVisibility(post.Visibility) {
		post.Visibility = models.VisibilityPublic
	}
	post.Published = c.FormValue("published") == "on"

	if err := models.DB.Save(&post).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update post")
	}

	c.Response().Header().Set("HX-Redirect", "/admin/dashboard")
	return c.NoContent(http.StatusOK)
}

func (h *BaseHandler) AdminPostDelete(c echo.Context) error {
	id, err := h.parseUintParam(c, "id")
	if err != nil {
		return err
	}

	if err := models.DB.Delete(&models.Post{}, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete post")
	}

	return c.NoContent(http.StatusOK)
}

// Helper for slug generation
func (h *BaseHandler) generateSlug(title string) string {
	return strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(regexp.MustCompile(`\s+`).ReplaceAllString(regexp.MustCompile(`[^a-z0-9\s-]`).ReplaceAllString(strings.ToLower(title), ""), "-"), "-"), "-")
}
