package models

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel contains common fields for all models
type BaseModel struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// Role constants and configuration
const (
	RoleUser    = "user"
	RoleAdmin   = "admin"
	RolePremium = "premium"
)

type RoleConfig struct {
	Valid bool
	Name  string
}

var Roles = map[string]RoleConfig{
	RoleUser:    {Valid: true, Name: "User"},
	RoleAdmin:   {Valid: true, Name: "Admin"},
	RolePremium: {Valid: true, Name: "Premium"},
}

// RoleNames provides backwards compatibility
var RoleNames = map[string]string{
	RoleUser: "User", RoleAdmin: "Admin", RolePremium: "Premium",
}

// Visibility constants and configuration
const (
	VisibilityPublic  = "public"
	VisibilityPremium = "premium"
	VisibilityAdmin   = "admin"
)

type VisibilityConfig struct {
	Valid bool
	Name  string
}

var Visibilities = map[string]VisibilityConfig{
	VisibilityPublic:  {Valid: true, Name: "Public"},
	VisibilityPremium: {Valid: true, Name: "Premium"},
	VisibilityAdmin:   {Valid: true, Name: "Admin"},
}

// VisibilityNames provides backwards compatibility
var VisibilityNames = map[string]string{
	VisibilityPublic: "Public", VisibilityPremium: "Premium", VisibilityAdmin: "Admin",
}

type Post struct {
	BaseModel
	Title      string `json:"title" gorm:"not null" validate:"required,min=1,max=255"`
	Content    string `json:"content" gorm:"type:text" validate:"required,min=1"`
	Slug       string `json:"slug" gorm:"unique;not null" validate:"required,min=1,max=255"`
	Published  bool   `json:"published" gorm:"default:false"`
	Visibility string `json:"visibility" gorm:"default:public" validate:"required,oneof=public premium admin"`
}

func (p *Post) CanAccess(user *User) bool {
	if !p.Published {
		return false
	}

	if p.Visibility == VisibilityPublic {
		return true
	}

	if user == nil {
		return false
	}

	if p.Visibility == VisibilityAdmin {
		return user.IsAdmin()
	}

	return user.IsPremium()
}

type User struct {
	BaseModel
	Email      string     `json:"email" gorm:"unique;not null" validate:"required,email"`
	Password   string     `json:"-" gorm:"not null" validate:"required,min=6"`
	Name       string     `json:"name" gorm:"not null" validate:"required,min=1,max=100"`
	Role       string     `json:"role" gorm:"default:user" validate:"required,oneof=user admin premium"`
	IsVerified bool       `json:"is_verified" gorm:"default:false"`
	OTP        string     `json:"-" gorm:"size:6"`
	OTPExpiry  *time.Time `json:"-"`
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsPremium() bool {
	return u.Role == RolePremium || u.IsAdmin()
}

// Helper functions for validation
func IsValidRole(role string) bool {
	_, exists := Roles[role]
	return exists
}

func IsValidVisibility(visibility string) bool {
	_, exists := Visibilities[visibility]
	return exists
}

// Get role display name
func GetRoleName(role string) string {
	if config, exists := Roles[role]; exists {
		return config.Name
	}
	return "Unknown"
}

// Get visibility display name
func GetVisibilityName(visibility string) string {
	if config, exists := Visibilities[visibility]; exists {
		return config.Name
	}
	return "Unknown"
}

// DashboardStats represents admin dashboard statistics
type DashboardStats struct {
	TotalUsers     int64
	PremiumUsers   int64
	TotalPosts     int64
	PublishedPosts int64
}
