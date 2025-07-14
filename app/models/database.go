package models

import (
	"fmt"
	"log"
	"mini-blog/app/config"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB(cfg *config.Config) {
	var dsnParts []string

	dsnParts = append(dsnParts, fmt.Sprintf("host=%s", cfg.DB.Host))
	dsnParts = append(dsnParts, fmt.Sprintf("user=%s", cfg.DB.User))
	dsnParts = append(dsnParts, fmt.Sprintf("dbname=%s", cfg.DB.Name))
	dsnParts = append(dsnParts, fmt.Sprintf("port=%s", cfg.DB.Port))
	dsnParts = append(dsnParts, fmt.Sprintf("sslmode=%s", cfg.DB.SSLMode))

	// Only add password if it's not empty
	if cfg.DB.Password != "" {
		dsnParts = append(dsnParts, fmt.Sprintf("password=%s", cfg.DB.Password))
	}

	dsn := strings.Join(dsnParts, " ")

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Connected to database")
}

func Migrate() {
	err := DB.AutoMigrate(&Post{}, &User{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
	log.Println("Database migrated successfully")
}

func CreateInitialAdmin(cfg *config.Config) {
	if cfg.Auth.AdminEmail == "" {
		return
	}

	var existingUser User
	if err := DB.Where("email = ?", cfg.Auth.AdminEmail).First(&existingUser).Error; err == nil {
		if !existingUser.IsAdmin() {
			DB.Model(&existingUser).Update("role", RoleAdmin)
			log.Printf("Updated user %s to admin", cfg.Auth.AdminEmail)
		}
		return
	}

	log.Printf("Admin email %s not found in database - user must sign up first", cfg.Auth.AdminEmail)
}
