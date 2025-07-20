package handlers

import (
	"fmt"
	"math/rand"
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// Unified handlers consolidating all functionality

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

// Auth handlers
func (h *BaseHandler) SignupPage(c echo.Context) error {
	return h.render(c, templates.Layout("Sign Up", templates.SignupForm(), c.Request().URL.Path))
}

func (h *BaseHandler) LoginPage(c echo.Context) error {
	return h.render(c, templates.Layout("Login", templates.LoginForm(), c.Request().URL.Path))
}

func (h *BaseHandler) Signup(c echo.Context) error {
	name := h.trimFormValue(c, "name")
	email := h.trimFormValue(c, "email")
	password := c.FormValue("password")
	confirmPassword := c.FormValue("confirm_password")

	// Validation
	if name == "" || email == "" || password == "" {
		return h.render(c, templates.SignupFormContent("All fields are required"))
	}
	if password != confirmPassword {
		return h.render(c, templates.SignupFormContent("Passwords do not match"))
	}
	if len(password) < 6 {
		return h.render(c, templates.SignupFormContent("Password must be at least 6 characters"))
	}

	// Check if user exists
	var existingUser models.User
	if err := models.DB.Where("email = ?", email).First(&existingUser).Error; err == nil {
		if existingUser.IsVerified {
			return h.render(c, templates.SignupFormContent("Account already exists. Please login."))
		}
		// User exists but not verified, update and resend OTP
		return h.updateAndResendOTP(c, &existingUser, name, password)
	}

	// Create new user
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to process password")
	}

	otp := h.generateOTP()
	otpExpiry := time.Now().Add(10 * time.Minute)

	user := models.User{
		Name:       name,
		Email:      email,
		Password:   string(hashedPassword),
		OTP:        otp,
		OTPExpiry:  &otpExpiry,
		IsVerified: false,
		Role:       models.RoleUser,
	}

	if err := models.DB.Create(&user).Error; err != nil {
		return h.render(c, templates.SignupFormContent("Email already registered"))
	}

	// Send OTP
	h.sendOTP(email, name, otp)

	return h.render(c, templates.OTPForm(email))
}

func (h *BaseHandler) Login(c echo.Context) error {
	email := h.trimFormValue(c, "email")
	password := c.FormValue("password")

	if email == "" || password == "" {
		return h.render(c, templates.LoginFormContent("Email and password are required"))
	}

	var user models.User
	if err := models.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return h.render(c, templates.LoginFormContent("Invalid email or password"))
	}

	if !user.IsVerified {
		return h.render(c, templates.LoginFormContent("Please verify your email before logging in"))
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return h.render(c, templates.LoginFormContent("Invalid email or password"))
	}

	h.setUserSession(c, user.ID)
	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

func (h *BaseHandler) VerifyOTP(c echo.Context) error {
	otp := h.trimFormValue(c, "otp")
	if otp == "" {
		return h.render(c, templates.OTPFormContent("", "Please enter the verification code"))
	}

	var user models.User
	if err := models.DB.Where("otp = ? AND otp_expiry > ?", otp, time.Now()).First(&user).Error; err != nil {
		return h.render(c, templates.OTPFormContent("", "Invalid or expired verification code"))
	}

	user.IsVerified, user.OTP, user.OTPExpiry = true, "", nil
	if user.Email == h.cfg.Auth.AdminEmail {
		user.Role = models.RoleAdmin
	}

	if err := models.DB.Save(&user).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to verify account")
	}

	h.emailService.SendWelcomeEmail(user.Email, user.Name, user.IsAdmin())
	h.setUserSession(c, user.ID)
	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

func (h *BaseHandler) ResendOTP(c echo.Context) error {
	return h.render(c, templates.OTPFormContent("", "OTP resent successfully"))
}

func (h *BaseHandler) Logout(c echo.Context) error {
	h.clearUserSession(c)
	return c.Redirect(http.StatusSeeOther, "/")
}

// Admin handlers
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

// Helper methods
func (h *BaseHandler) generateOTP() string {
	rand.Seed(time.Now().UnixNano())
	otp := rand.Intn(900000) + 100000
	return strconv.Itoa(otp)
}

func (h *BaseHandler) generateSlug(title string) string {
	return strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(regexp.MustCompile(`\s+`).ReplaceAllString(regexp.MustCompile(`[^a-z0-9\s-]`).ReplaceAllString(strings.ToLower(title), ""), "-"), "-"), "-")
}

func (h *BaseHandler) sendOTP(email, name, otp string) {
	if err := h.emailService.SendOTP(email, name, otp); err != nil {
		fmt.Printf("Failed to send OTP email: %v\n", err)
		fmt.Printf("OTP for %s: %s\n", email, otp)
	}
}

func (h *BaseHandler) updateAndResendOTP(c echo.Context, user *models.User, name, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to process password")
	}

	otp := h.generateOTP()
	otpExpiry := time.Now().Add(10 * time.Minute)

	user.Name = name
	user.Password = string(hashedPassword)
	user.OTP = otp
	user.OTPExpiry = &otpExpiry

	if err := models.DB.Save(user).Error; err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update account")
	}

	h.sendOTP(user.Email, name, otp)
	return h.render(c, templates.OTPForm(user.Email))
}
