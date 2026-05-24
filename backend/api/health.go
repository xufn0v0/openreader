package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"app":       "openreader",
		"version":   Version,
		"commit":    Commit,
		"buildDate": BuildDate,
	})
}
