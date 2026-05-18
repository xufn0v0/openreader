package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

func (s *Server) cacheStats(c *gin.Context) {
	fileCount, totalSize := directoryStats(s.cfg.CacheDir)

	var cachedChapters int64
	if err := s.db.Model(&models.Chapter{}).Where("cache_path <> ''").Count(&cachedChapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count cached chapters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":           s.cfg.CacheDir,
		"files":          fileCount,
		"size":           totalSize,
		"cachedChapters": cachedChapters,
	})
}

func (s *Server) clearCache(c *gin.Context) {
	cacheDir, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil || cacheDir == string(os.PathSeparator) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid cache directory"})
		return
	}

	files, size := directoryStats(cacheDir)
	if err := os.RemoveAll(cacheDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cache"})
		return
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to recreate cache directory"})
		return
	}
	if err := s.db.Model(&models.Chapter{}).Where("cache_path <> ''").Update("cache_path", "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset chapter cache state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"clearedFiles": files, "clearedSize": size})
}

func directoryStats(root string) (int, int64) {
	var fileCount int
	var totalSize int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		fileCount++
		totalSize += info.Size()
		return nil
	})
	return fileCount, totalSize
}
