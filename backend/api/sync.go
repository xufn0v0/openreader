package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"openreader/backend/middleware"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

func (s *Server) syncSocket(c *gin.Context) {
	token := c.Query("token")
	userID, err := middleware.ParseToken(s.cfg.JWTSecret, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := s.hub.AddClient(userID, conn)
	go client.WritePump()
	client.ReadPump()
}
