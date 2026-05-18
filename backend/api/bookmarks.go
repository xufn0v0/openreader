package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

type bookmarkRequest struct {
	ChapterID    uint    `json:"chapterId"`
	ChapterIndex int     `json:"chapterIndex"`
	Offset       int     `json:"offset"`
	Percent      float64 `json:"percent"`
	Title        string  `json:"title"`
	Excerpt      string  `json:"excerpt"`
	Note         string  `json:"note"`
}

func (s *Server) listBookmarks(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var bookmarks []models.Bookmark
	if err := s.db.Where("user_id = ? AND book_id = ?", userID, bookID).
		Order("chapter_index asc, offset asc, created_at asc").
		Find(&bookmarks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list bookmarks"})
		return
	}
	c.JSON(http.StatusOK, bookmarks)
}

func (s *Server) createBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if _, ok := s.ensureBook(c, userID, bookID); !ok {
		return
	}

	var request bookmarkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark payload"})
		return
	}

	bookmark := models.Bookmark{
		UserID:       userID,
		BookID:       bookID,
		ChapterID:    request.ChapterID,
		ChapterIndex: request.ChapterIndex,
		Offset:       request.Offset,
		Percent:      request.Percent,
		Title:        strings.TrimSpace(request.Title),
		Excerpt:      strings.TrimSpace(request.Excerpt),
		Note:         strings.TrimSpace(request.Note),
	}
	if bookmark.Title == "" {
		bookmark.Title = "书签"
	}

	if err := s.db.Create(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create bookmark"})
		return
	}
	c.JSON(http.StatusCreated, bookmark)
}

func (s *Server) updateBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookmarkID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var bookmark models.Bookmark
	if err := s.db.Where("user_id = ? AND id = ?", userID, bookmarkID).First(&bookmark).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}

	var request bookmarkRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark payload"})
		return
	}

	if title := strings.TrimSpace(request.Title); title != "" {
		bookmark.Title = title
	}
	bookmark.Excerpt = strings.TrimSpace(request.Excerpt)
	bookmark.Note = strings.TrimSpace(request.Note)

	if err := s.db.Save(&bookmark).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update bookmark"})
		return
	}
	c.JSON(http.StatusOK, bookmark)
}

func (s *Server) deleteBookmark(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	bookmarkID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	result := s.db.Where("user_id = ? AND id = ?", userID, bookmarkID).Delete(&models.Bookmark{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete bookmark"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return
	}
	c.Status(http.StatusNoContent)
}
