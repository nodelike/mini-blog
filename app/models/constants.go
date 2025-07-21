package models

// User roles
const (
	RoleAdmin   = "admin"
	RolePremium = "premium"
	RoleUser    = "user"
)

// Post visibility levels
const (
	VisibilityPublic  = "public"
	VisibilityPremium = "premium"
	VisibilityAdmin   = "admin"
)

// Media types
const (
	MediaTypeTV    = "tv"
	MediaTypeMovie = "movie"
)

// Media tracking statuses
const (
	StatusWatching  = "watching"
	StatusCompleted = "completed"
	StatusPlanned   = "planned"
	StatusDropped   = "dropped"
)

// Validation maps
var (
	ValidRoles = map[string]bool{
		RoleAdmin:   true,
		RolePremium: true,
		RoleUser:    true,
	}

	ValidVisibilities = map[string]bool{
		VisibilityPublic:  true,
		VisibilityPremium: true,
		VisibilityAdmin:   true,
	}

	ValidMediaTypes = map[string]bool{
		MediaTypeTV:    true,
		MediaTypeMovie: true,
	}

	ValidStatuses = map[string]bool{
		StatusWatching:  true,
		StatusCompleted: true,
		StatusPlanned:   true,
		StatusDropped:   true,
	}

	RoleNames = map[string]string{
		RoleAdmin:   "Admin",
		RolePremium: "Premium",
		RoleUser:    "User",
	}
)

// Validation functions
func IsValidRole(role string) bool      { return ValidRoles[role] }
func IsValidVisibility(vis string) bool { return ValidVisibilities[vis] }
func IsValidMediaType(mt string) bool   { return ValidMediaTypes[mt] }
func IsValidStatus(status string) bool  { return ValidStatuses[status] }
func GetRoleName(role string) string    { return RoleNames[role] }
