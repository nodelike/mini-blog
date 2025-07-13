package handlers

import (
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type AdminHandler struct {
}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

func (h *AdminHandler) Dashboard(c echo.Context, user *models.User) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		return render(c, templates.AdminDashboard())
	}

	return render(c, templates.AdminLayout("Admin Dashboard", templates.AdminDashboard(), user))
}

func (h *AdminHandler) UsersList(c echo.Context, user *models.User) error {
	var users []models.User
	if err := models.DB.Order("created_at desc").Find(&users).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch users")
	}

	return render(c, templates.AdminUsersList(users))
}

func (h *AdminHandler) UpdateUserRole(c echo.Context, user *models.User) error {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	newRole := c.FormValue("role")
	if newRole == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Role is required")
	}

	if newRole != models.RoleUser && newRole != models.RoleAdmin && newRole != models.RolePremium {
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

	return render(c, templates.AdminUserRow(targetUser))
}

func (h *AdminHandler) Stats(c echo.Context, user *models.User) error {
	return render(c, templates.AdminStats())
}
