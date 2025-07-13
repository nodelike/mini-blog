package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DB struct {
		Host, Port, User, Password, Name, SSLMode string
	}
	JWT     struct{ Secret string }
	Session struct{ Key string }
	Server  struct{ Port string }
	Auth    struct{ AdminEmail, ResendAPIKey string }
	Env     string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	defaults := map[string]string{
		"DB_HOST":     "localhost",
		"DB_PORT":     "5432",
		"DB_USER":     "postgres",
		"DB_PASSWORD": "password",
		"DB_NAME":     "mini_blog",
		"DB_SSL_MODE": "disable",
		"JWT_SECRET":  "your-secret-key-change-this-in-production",
		"SESSION_KEY": "your-session-secret-32-characters-long",
		"PORT":        "8080",
		"ENV":         "development",
	}

	getEnv := func(key string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return defaults[key]
	}

	cfg := &Config{}

	cfg.DB.Host = getEnv("DB_HOST")
	cfg.DB.Port = getEnv("DB_PORT")
	cfg.DB.User = getEnv("DB_USER")
	cfg.DB.Password = getEnv("DB_PASSWORD")
	cfg.DB.Name = getEnv("DB_NAME")
	cfg.DB.SSLMode = getEnv("DB_SSL_MODE")

	cfg.JWT.Secret = getEnv("JWT_SECRET")
	cfg.Session.Key = getEnv("SESSION_KEY")
	cfg.Server.Port = getEnv("PORT")
	cfg.Auth.AdminEmail = os.Getenv("ADMIN_EMAIL")
	cfg.Auth.ResendAPIKey = os.Getenv("RESEND_API_KEY")
	cfg.Env = getEnv("ENV")

	return cfg
}
