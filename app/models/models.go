package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	RoleUser    = "user"
	RoleAdmin   = "admin"
	RolePremium = "premium"

	VisibilityPublic  = "public"
	VisibilityPremium = "premium"
	VisibilityAdmin   = "admin"
)

var (
	ValidRoles = map[string]bool{
		RoleUser: true, RoleAdmin: true, RolePremium: true,
	}
	ValidVisibilities = map[string]bool{
		VisibilityPublic: true, VisibilityPremium: true, VisibilityAdmin: true,
	}

	RoleNames = map[string]string{
		RoleUser: "User", RoleAdmin: "Admin", RolePremium: "Premium",
	}

	VisibilityNames = map[string]string{
		VisibilityPublic: "Public", VisibilityPremium: "Premium", VisibilityAdmin: "Admin",
	}
)

type Post struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	Title      string         `json:"title" gorm:"not null" validate:"required,min=1,max=255"`
	Content    string         `json:"content" gorm:"type:text" validate:"required,min=1"`
	Slug       string         `json:"slug" gorm:"unique;not null" validate:"required,min=1,max=255"`
	Published  bool           `json:"published" gorm:"default:false"`
	Visibility string         `json:"visibility" gorm:"default:public" validate:"required,oneof=public premium admin"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"index"`
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
	ID         uint           `json:"id" gorm:"primaryKey"`
	Email      string         `json:"email" gorm:"unique;not null" validate:"required,email"`
	Password   string         `json:"-" gorm:"not null" validate:"required,min=6"`
	Name       string         `json:"name" gorm:"not null" validate:"required,min=1,max=100"`
	Role       string         `json:"role" gorm:"default:user" validate:"required,oneof=user admin premium"`
	IsVerified bool           `json:"is_verified" gorm:"default:false"`
	OTP        string         `json:"-" gorm:"size:6"`
	OTPExpiry  *time.Time     `json:"-"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u *User) IsPremium() bool {
	return u.Role == RolePremium || u.IsAdmin()
}
