package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) uploadAsset(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	if fileHeader.Size > 8*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is too large"})
		return
	}

	kind := strings.TrimSpace(c.PostForm("type"))
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedUploadExtension(kind, ext) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	dir := filepath.Join(s.cfg.DataDir, "uploads", uploadKindDir(kind))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload directory"})
		return
	}
	name := time.Now().Format("20060102150405") + "-" + randomHex(6) + ext
	target := filepath.Join(dir, name)
	if err := c.SaveUploadedFile(fileHeader, target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"url":  "/uploads/" + uploadKindDir(kind) + "/" + name,
		"name": fileHeader.Filename,
		"size": fileHeader.Size,
		"type": uploadKindDir(kind),
	})
}

func uploadKindDir(kind string) string {
	switch kind {
	case "cover":
		return "covers"
	case "background":
		return "backgrounds"
	case "font":
		return "fonts"
	default:
		return "misc"
	}
}

func allowedUploadExtension(kind, ext string) bool {
	imageExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	fontExts := map[string]bool{".ttf": true, ".otf": true, ".woff": true, ".woff2": true}
	switch kind {
	case "cover", "background":
		return imageExts[ext]
	case "font":
		return fontExts[ext]
	default:
		return imageExts[ext] || fontExts[ext]
	}
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	return hex.EncodeToString(buf)
}
