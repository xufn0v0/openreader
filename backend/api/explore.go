package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func (s *Server) listExploreSources(c *gin.Context) {
	var sources []models.BookSource
	if err := s.db.Where("enabled = ?", true).Order("name asc").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}
	out := make([]models.BookSource, 0)
	for _, source := range sources {
		rule, err := source.ParsedRules()
		if err == nil && strings.TrimSpace(rule.ExploreURL) != "" {
			out = append(out, source)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) exploreBooks(c *gin.Context) {
	sourceID, ok := parseUintParam(c, "sourceId")
	if !ok {
		return
	}
	var source models.BookSource
	if err := s.db.Where("enabled = ?", true).First(&source, sourceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page"})
			return
		}
		page = parsed
	}
	results, err := engine.ExploreBooksPage(source, page)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}
