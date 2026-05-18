package engine

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
)

func ChapterCachePath(bookURL, chapterURL string) string {
	sum := md5.Sum([]byte(bookURL + "\n" + chapterURL))
	encoded := hex.EncodeToString(sum[:])
	return filepath.Join(encoded[:2], encoded[2:])
}

func WriteChapterCache(cacheDir, bookURL, chapterURL, content string) (string, error) {
	relativePath := ChapterCachePath(bookURL, chapterURL)
	fullPath := filepath.Join(cacheDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return relativePath, nil
}

func ReadChapterCache(cacheDir, relativePath string) (string, error) {
	content, err := os.ReadFile(filepath.Join(cacheDir, relativePath))
	if err != nil {
		return "", err
	}
	return string(content), nil
}
