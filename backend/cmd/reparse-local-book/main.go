package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"openreader/backend/engine"
)

type chapterRow struct {
	Index     int
	Title     string
	CachePath string
}

func main() {
	dbPath := flag.String("db", "../data/openreader.db", "SQLite database path")
	cacheDir := flag.String("cache", "../cache", "chapter cache directory")
	bookID := flag.Uint("book", 0, "book id to reparse")
	flag.Parse()

	if *bookID == 0 {
		log.Fatal("-book is required")
	}

	database, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	var bookURL string
	if err := database.QueryRow("select url from books where id = ?", *bookID).Scan(&bookURL); err != nil {
		log.Fatalf("load book: %v", err)
	}

	chapters, err := loadCurrentChapters(database, *bookID)
	if err != nil {
		log.Fatal(err)
	}
	if len(chapters) == 0 {
		log.Fatal("book has no chapters")
	}

	reconstructed, err := reconstructText(*cacheDir, chapters)
	if err != nil {
		log.Fatal(err)
	}

	parsed, err := engine.ParseTXT([]byte(reconstructed))
	if err != nil {
		log.Fatal(err)
	}
	if len(parsed) == 0 {
		log.Fatal("reparse produced no chapters")
	}

	tx, err := database.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("delete from bookmarks where book_id = ?", *bookID); err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Exec("delete from reading_progresses where book_id = ?", *bookID); err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Exec("delete from chapters where book_id = ?", *bookID); err != nil {
		log.Fatal(err)
	}

	for index, chapter := range parsed {
		chapterURL := fmt.Sprintf("%s/chapter_%d", bookURL, index)
		cachePath, err := engine.WriteChapterCache(*cacheDir, bookURL, chapterURL, chapter.Content)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := tx.Exec(
			"insert into chapters (book_id, `index`, title, url, cache_path, created_at, updated_at) values (?, ?, ?, ?, ?, datetime('now'), datetime('now'))",
			*bookID,
			index,
			chapter.Title,
			chapterURL,
			cachePath,
		); err != nil {
			log.Fatal(err)
		}
	}

	lastTitle := parsed[len(parsed)-1].Title
	if _, err := tx.Exec("update books set chapter_count = ?, last_chapter = ?, updated_at = datetime('now') where id = ?", len(parsed), lastTitle, *bookID); err != nil {
		log.Fatal(err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("reparsed book %d into %d chapters\n", *bookID, len(parsed))
}

func loadCurrentChapters(database *sql.DB, bookID uint) ([]chapterRow, error) {
	rows, err := database.Query("select `index`, title, cache_path from chapters where book_id = ? order by `index` asc", bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chapters := make([]chapterRow, 0)
	for rows.Next() {
		var chapter chapterRow
		if err := rows.Scan(&chapter.Index, &chapter.Title, &chapter.CachePath); err != nil {
			return nil, err
		}
		chapters = append(chapters, chapter)
	}
	return chapters, rows.Err()
}

func reconstructText(cacheDir string, chapters []chapterRow) (string, error) {
	var builder strings.Builder
	for _, chapter := range chapters {
		content, err := os.ReadFile(filepath.Join(cacheDir, chapter.CachePath))
		if err != nil {
			return "", err
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		if chapter.Title != "正文" {
			builder.WriteString(chapter.Title)
			builder.WriteString("\n")
		}
		builder.Write(content)
	}
	return builder.String(), nil
}
