package config

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DB struct {
		Host     string `envconfig:"DB_HOST" default:"localhost"`
		Port     string `envconfig:"DB_PORT" default:"5432"`
		User     string `envconfig:"DB_USER" default:"postgres"`
		Password string `envconfig:"DB_PASSWORD" default:"password"`
		Name     string `envconfig:"DB_NAME" default:"mini_blog"`
		SSLMode  string `envconfig:"DB_SSL_MODE" default:"disable"`
	}
	JWT struct {
		Secret string `envconfig:"JWT_SECRET" default:"your-secret-key-change-this-in-production"`
	}
	Session struct {
		Key string `envconfig:"SESSION_KEY" default:"your-session-secret-32-characters-long"`
	}
	Server struct {
		Port string `envconfig:"PORT" default:"8080"`
	}
	Auth struct {
		AdminEmail   string `envconfig:"ADMIN_EMAIL"`
		ResendAPIKey string `envconfig:"RESEND_API_KEY"`
	}
	Env string `envconfig:"ENV" default:"development"`
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatal("Error processing environment variables:", err)
	}

	return &cfg
}
