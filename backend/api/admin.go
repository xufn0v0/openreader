package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
)

func (s *Server) requireAdmin(c *gin.Context) bool {
	userID, ok := middleware.UserID(c)
	if !ok {
		unauthorized(c, "login required")
		return false
	}
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil || user.Role != "admin" {
		c.JSON(http.StatusForbidden, errResp("FORBIDDEN", "admin access required"))
		return false
	}
	return true
}

func (s *Server) listUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	var users []models.User
	if err := s.db.Order("created_at desc").Find(&users).Error; err != nil {
		internalError(c, "failed to list users")
		return
	}

	type userSummary struct {
		ID             uint      `json:"id"`
		Username       string    `json:"username"`
		Role           string    `json:"role"`
		BookLimit      int       `json:"bookLimit"`
		SourceLimit    int       `json:"sourceLimit"`
		CanEditSources bool      `json:"canEditSources"`
		CanAccessStore bool      `json:"canAccessStore"`
		BookCount      int64     `json:"bookCount"`
		LastActiveAt   time.Time `json:"lastActiveAt"`
		CreatedAt      time.Time `json:"createdAt"`
	}

	results := make([]userSummary, 0, len(users))
	for _, u := range users {
		var bookCount int64
		_ = s.db.Model(&models.Book{}).Where("user_id = ?", u.ID).Count(&bookCount).Error
		results = append(results, userSummary{
			ID:             u.ID,
			Username:       u.Username,
			Role:           u.Role,
			BookLimit:      u.BookLimit,
			SourceLimit:    u.SourceLimit,
			CanEditSources: u.CanEditSources,
			CanAccessStore: u.CanAccessStore,
			BookCount:      bookCount,
			LastActiveAt:   u.LastActiveAt,
			CreatedAt:      u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, results)
}

type updateUserRequest struct {
	BookLimit      *int  `json:"bookLimit"`
	SourceLimit    *int  `json:"sourceLimit"`
	CanEditSources *bool `json:"canEditSources"`
	CanAccessStore *bool `json:"canAccessStore"`
}

func (s *Server) updateUser(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		notFound(c, "user not found")
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "invalid payload")
		return
	}

	if req.BookLimit != nil {
		user.BookLimit = *req.BookLimit
	}
	if req.SourceLimit != nil {
		user.SourceLimit = *req.SourceLimit
	}
	if req.CanEditSources != nil {
		user.CanEditSources = *req.CanEditSources
	}
	if req.CanAccessStore != nil {
		user.CanAccessStore = *req.CanAccessStore
	}

	if err := s.db.Save(&user).Error; err != nil {
		internalError(c, "failed to update user")
		return
	}
	c.JSON(http.StatusOK, user)
}

func (s *Server) cleanupInactiveUsers(c *gin.Context) {
	if !s.requireAdmin(c) {
		return
	}

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	result := s.db.Where("role != ? AND last_active_at < ?", "admin", cutoff).Delete(&models.User{})
	if result.Error != nil {
		internalError(c, "cleanup failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": result.RowsAffected})
}
