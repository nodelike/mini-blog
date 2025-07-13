package handlers

import (
	"mini-blog/app/config"
	"mini-blog/app/models"
	"mini-blog/app/services"
	"mini-blog/app/templates"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	validator    *validator.Validate
	emailService *services.EmailService
	store        *sessions.CookieStore
	cfg          *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	store := sessions.NewCookieStore([]byte(cfg.Session.Key))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   cfg.Env == "production",
	}

	return &AuthHandler{
		validator:    validator.New(),
		emailService: services.NewEmailService(cfg),
		store:        store,
		cfg:          cfg,
	}
}

func (h *AuthHandler) SignupPage(c echo.Context) error {
	return render(c, templates.Layout("Sign Up", templates.SignupForm()))
}

func (h *AuthHandler) LoginPage(c echo.Context) error {
	return render(c, templates.Layout("Login", templates.LoginForm()))
}

func (h *AuthHandler) Signup(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	email := strings.TrimSpace(c.FormValue("email"))
	password := c.FormValue("password")
	confirmPassword := c.FormValue("confirm_password")

	if name == "" || email == "" || password == "" {
		return render(c, templates.SignupFormContent("All fields are required"))
	}

	if password != confirmPassword {
		return render(c, templates.SignupFormContent("Passwords do not match"))
	}

	if len(password) < 6 {
		return render(c, templates.SignupFormContent("Password must be at least 6 characters"))
	}

	var existingUser models.User
	if err := models.DB.Where("email = ?", email).First(&existingUser).Error; err == nil {
		if !existingUser.IsVerified {
			otp := services.GenerateOTP()
			otpExpiry := time.Now().Add(10 * time.Minute)

			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return render(c, templates.SignupFormContent("Failed to process password"))
			}

			existingUser.Name = name
			existingUser.Password = string(hashedPassword)
			existingUser.OTP = otp
			existingUser.OTPExpiry = &otpExpiry

			if err := models.DB.Save(&existingUser).Error; err != nil {
				return render(c, templates.SignupFormContent("Failed to update account"))
			}

			if err := h.emailService.SendOTP(email, name, otp); err != nil {
				c.Logger().Errorf("Failed to send OTP email: %v", err)
			}

			session, _ := h.store.Get(c.Request(), "auth-session")
			session.Values["pending_user_id"] = existingUser.ID
			session.Save(c.Request(), c.Response())

			return render(c, templates.OTPFormContent(email))
		}

		return render(c, templates.SignupFormContent("Email already registered"))
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return render(c, templates.SignupFormContent("Failed to process password"))
	}

	otp := services.GenerateOTP()
	otpExpiry := time.Now().Add(10 * time.Minute)

	user := models.User{
		Name:       name,
		Email:      email,
		Password:   string(hashedPassword),
		Role:       models.RoleUser,
		IsVerified: false,
		OTP:        otp,
		OTPExpiry:  &otpExpiry,
	}

	if err := models.DB.Create(&user).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return render(c, templates.SignupFormContent("Email already registered"))
		}
		return render(c, templates.SignupFormContent("Failed to create account"))
	}

	if err := h.emailService.SendOTP(email, name, otp); err != nil {
		c.Logger().Errorf("Failed to send OTP email: %v", err)
	}

	session, _ := h.store.Get(c.Request(), "auth-session")
	session.Values["pending_user_id"] = user.ID
	session.Save(c.Request(), c.Response())

	return render(c, templates.OTPFormContent(email))
}

func (h *AuthHandler) VerifyOTP(c echo.Context) error {
	otp := strings.TrimSpace(c.FormValue("otp"))

	if otp == "" {
		return render(c, templates.OTPFormContent("", "OTP is required"))
	}

	session, _ := h.store.Get(c.Request(), "auth-session")
	userID, ok := session.Values["pending_user_id"].(uint)
	if !ok {
		return render(c, templates.OTPFormContent("", "Session expired. Please sign up again."))
	}

	var user models.User
	if err := models.DB.First(&user, userID).Error; err != nil {
		return render(c, templates.OTPFormContent("", "Invalid session. Please sign up again."))
	}

	if user.OTP != otp {
		return render(c, templates.OTPFormContent(user.Email, "Invalid OTP"))
	}

	if user.OTPExpiry == nil || time.Now().After(*user.OTPExpiry) {
		return render(c, templates.OTPFormContent(user.Email, "OTP has expired"))
	}

	user.IsVerified = true
	user.OTP = ""
	user.OTPExpiry = nil

	if err := models.DB.Save(&user).Error; err != nil {
		return render(c, templates.OTPFormContent(user.Email, "Failed to verify account"))
	}

	if err := h.emailService.SendWelcomeEmail(user.Email, user.Name, user.IsAdmin()); err != nil {
		c.Logger().Errorf("Failed to send welcome email: %v", err)
	}

	delete(session.Values, "pending_user_id")
	session.Values["user_id"] = user.ID
	session.Save(c.Request(), c.Response())

	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

func (h *AuthHandler) ResendOTP(c echo.Context) error {
	session, _ := h.store.Get(c.Request(), "auth-session")
	userID, ok := session.Values["pending_user_id"].(uint)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending verification"})
	}

	var user models.User
	if err := models.DB.First(&user, userID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User not found"})
	}

	otp := services.GenerateOTP()
	otpExpiry := time.Now().Add(10 * time.Minute)

	user.OTP = otp
	user.OTPExpiry = &otpExpiry

	if err := models.DB.Save(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate new OTP"})
	}

	if err := h.emailService.SendOTP(user.Email, user.Name, otp); err != nil {
		c.Logger().Errorf("Failed to resend OTP email: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to send OTP"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "OTP resent successfully"})
}

func (h *AuthHandler) Login(c echo.Context) error {
	email := strings.TrimSpace(c.FormValue("email"))
	password := c.FormValue("password")

	if email == "" || password == "" {
		return render(c, templates.LoginFormContent("Email and password are required"))
	}

	var user models.User
	if err := models.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return render(c, templates.LoginFormContent("Invalid email or password"))
	}

	if !user.IsVerified {
		return render(c, templates.LoginFormContent("Please verify your email before logging in"))
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return render(c, templates.LoginFormContent("Invalid email or password"))
	}

	session, _ := h.store.Get(c.Request(), "auth-session")
	session.Values["user_id"] = user.ID
	session.Save(c.Request(), c.Response())

	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

func (h *AuthHandler) Logout(c echo.Context) error {
	session, _ := h.store.Get(c.Request(), "auth-session")
	session.Values["user_id"] = nil
	session.Options.MaxAge = -1
	session.Save(c.Request(), c.Response())

	return c.Redirect(http.StatusSeeOther, "/")
}

func (h *AuthHandler) GetCurrentUser(c echo.Context) *models.User {
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

func (h *AuthHandler) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := h.GetCurrentUser(c)
		if user == nil {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		c.Set("user", user)
		return next(c)
	}
}

func (h *AuthHandler) RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := h.GetCurrentUser(c)
		if user == nil || !user.IsAdmin() {
			return echo.NewHTTPError(http.StatusForbidden, "Admin access required")
		}
		c.Set("user", user)
		return next(c)
	}
}

func (h *AuthHandler) RequirePremium(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := h.GetCurrentUser(c)
		if user == nil || !user.IsPremium() {
			return echo.NewHTTPError(http.StatusForbidden, "Premium access required")
		}
		c.Set("user", user)
		return next(c)
	}
}
