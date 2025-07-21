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

type Media struct {
	BaseModel
	TMDBID      int        `json:"tmdb_id" gorm:"uniqueIndex;not null"`
	Type        string     `json:"type" gorm:"not null" validate:"required,oneof=movie tv"`
	Title       string     `json:"title" gorm:"not null" validate:"required"`
	Overview    string     `json:"overview" gorm:"type:text"`
	PosterPath  string     `json:"poster_path"`
	ReleaseDate *time.Time `json:"release_date"`
	Genres      string     `json:"genres" gorm:"type:text"` // JSON string of genres
	Popularity  float64    `json:"popularity"`
	VoteCount   int        `json:"vote_count"`
	VoteAverage float64    `json:"vote_average"`
	IsAnime     bool       `json:"is_anime" gorm:"default:false"`

	// Single user tracking fields
	Status        string     `json:"status" gorm:"default:planned" validate:"oneof=watching completed planned dropped"`
	Progress      int        `json:"progress"`       // episodes watched for TV
	TotalEpisodes int        `json:"total_episodes"` // total episodes (cached from TMDB)
	Rating        float64    `json:"rating" validate:"min=0,max=10"`
	Notes         string     `json:"notes" gorm:"type:text"`
	AddedAt       time.Time  `json:"added_at" gorm:"autoCreateTime"`
	LastSyncedAt  *time.Time `json:"last_synced_at" gorm:"index"`
}

// Episode model to store complete episode data locally with single-user tracking
type Episode struct {
	BaseModel
	TMDBID        int        `json:"tmdb_id" gorm:"index;not null"` // Show's TMDB ID
	SeasonNumber  int        `json:"season_number" gorm:"not null"`
	EpisodeNumber int        `json:"episode_number" gorm:"not null"`
	Name          string     `json:"name" gorm:"not null"`
	Overview      string     `json:"overview" gorm:"type:text"`
	AirDate       *time.Time `json:"air_date"`
	Runtime       int        `json:"runtime"`    // Runtime in minutes
	StillPath     string     `json:"still_path"` // Episode screenshot
	VoteAverage   float64    `json:"vote_average"`
	VoteCount     int        `json:"vote_count"`

	// Single user tracking fields
	Watched   bool       `json:"watched" gorm:"default:false"`
	WatchedAt *time.Time `json:"watched_at"`
}

// Season model to store season data locally
type Season struct {
	BaseModel
	TMDBID       int        `json:"tmdb_id" gorm:"index;not null"` // Show's TMDB ID
	SeasonNumber int        `json:"season_number" gorm:"not null"`
	Name         string     `json:"name" gorm:"not null"`
	Overview     string     `json:"overview" gorm:"type:text"`
	AirDate      *time.Time `json:"air_date"`
	EpisodeCount int        `json:"episode_count"`
	PosterPath   string     `json:"poster_path"`
}

// DashboardStats for admin dashboard
type DashboardStats struct {
	TotalUsers     int64
	PremiumUsers   int64
	TotalPosts     int64
	PublishedPosts int64
}
