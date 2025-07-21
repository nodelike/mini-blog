package models

import (
	"fmt"
	"log"
	"mini-blog/app/config"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB(cfg *config.Config) {
	dsn := fmt.Sprintf("host=%s user=%s dbname=%s port=%s sslmode=%s",
		cfg.DB.Host, cfg.DB.User, cfg.DB.Name, cfg.DB.Port, cfg.DB.SSLMode)

	if cfg.DB.Password != "" {
		dsn += fmt.Sprintf(" password=%s", cfg.DB.Password)
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Connected to database")
}

func RunMigrations() {
	if err := DB.AutoMigrate(&User{}, &Post{}, &Media{}, &Episode{}, &Season{}); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed successfully")
}

func CreateInitialAdmin(cfg *config.Config) {
	var count int64
	DB.Model(&User{}).Count(&count)

	// Create admin user if no users exist
	if count == 0 && cfg.Auth.AdminEmail != "" {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)

		admin := User{
			Name:       "Admin",
			Email:      cfg.Auth.AdminEmail,
			Password:   string(hashedPassword),
			IsVerified: true,
			Role:       RoleAdmin,
		}

		if err := DB.Create(&admin).Error; err != nil {
			log.Printf("Failed to create admin user: %v", err)
		} else {
			log.Printf("Admin user created: %s (password: admin123)", cfg.Auth.AdminEmail)
		}
	}
}
