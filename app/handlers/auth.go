package handlers

import (
	"fmt"
	"math/rand"
	"mini-blog/app/models"
	"mini-blog/app/templates"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// Auth page handlers
func (h *BaseHandler) SignupPage(c echo.Context) error {
	return h.render(c, templates.Layout("Sign Up", templates.SignupForm(), c.Request().URL.Path))
}

func (h *BaseHandler) LoginPage(c echo.Context) error {
	return h.render(c, templates.Layout("Login", templates.LoginForm(), c.Request().URL.Path))
}

// Auth action handlers
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

	// For successful signup, change HTMX target to replace entire form wrapper
	c.Response().Header().Set("HX-Retarget", "#auth-form-wrapper")
	c.Response().Header().Set("HX-Reswap", "outerHTML")
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

// Helper methods for auth
func (h *BaseHandler) generateOTP() string {
	rand.Seed(time.Now().UnixNano())
	otp := rand.Intn(900000) + 100000
	return strconv.Itoa(otp)
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

	// For successful signup, change HTMX target to replace entire form wrapper
	c.Response().Header().Set("HX-Retarget", "#auth-form-wrapper")
	c.Response().Header().Set("HX-Reswap", "outerHTML")
	return h.render(c, templates.OTPForm(user.Email))
}
