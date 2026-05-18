package db

import (
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return nil, err
	}

	database, err := gorm.Open(sqlite.Open(cfg.DatabasePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, statement := range pragmas {
		if err := database.Exec(statement).Error; err != nil {
			return nil, err
		}
	}

	return database, nil
}

func AutoMigrate(database *gorm.DB) error {
	return database.AutoMigrate(
		&models.User{},
		&models.BookSource{},
		&models.ReplaceRule{},
		&models.RSSSource{},
		&models.RSSArticle{},
		&models.Category{},
		&models.Book{},
		&models.Chapter{},
		&models.ReadingProgress{},
		&models.Bookmark{},
	)
}
