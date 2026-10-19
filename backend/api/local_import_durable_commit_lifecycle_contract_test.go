package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

const durableImportTitle = "持久提交测试书"
const durableImportAuthor = "测试作者"
const durableImportText = "第一章 原章\nowned-durable-import-paragraph\n第二章 后章\nowned-second-paragraph"

func durableImportConfirmRoutes() []stageLifecycleRoute {
	var routes []stageLifecycleRoute
	for _, route := range stageLifecycleRoutes() {
		if !route.preview {
			routes = append(routes, route)
		}
	}
	return routes
}

func durableImportCategories(t *testing.T, server *Server) []uint {
	t.Helper()
	var ids []uint
	for _, name := range []string{"old-group", "selected-first", "selected-second"} {
		category := models.Category{UserID: 1, Name: name}
		if err := server.db.Create(&category).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, category.ID)
	}
	return ids
}

func durableImportPlaceholder(t *testing.T, server *Server, categoryID uint) models.Book {
	t.Helper()
	book := models.Book{
		UserID: 1, SourceID: 0, Title: durableImportTitle, Author: durableImportAuthor,
		URL: "storage/data/default/durable.txt", CategoryID: &categoryID, ChapterCount: 2,
		Intro: "existing metadata", CanUpdate: true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&models.BookCategory{UserID: 1, BookID: book.ID, CategoryID: categoryID},
		&models.ReadingProgress{UserID: 1, BookID: book.ID, ChapterIndex: 0, ChapterTitle: "第一章 原章", Offset: 7},
		&models.Bookmark{UserID: 1, BookID: book.ID, ChapterIndex: 0, Offset: 9, Title: "existing bookmark"},
	} {
		if err := server.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return book
}

func durableImportRequest(t *testing.T, server *Server, route stageLifecycleRoute, auth string, staged bool, categoryIDs []uint) (*http.Request, *httptest.ResponseRecorder, string) {
	t.Helper()
	token := ""
	if staged {
		var err error
		token, err = server.stageLocalImport(1, "durable.txt", ".txt", []byte(durableImportText))
		if err != nil {
			t.Fatal(err)
		}
	}
	if route.direct {
		parts := []directLocalImportMultipartPart{
			{name: "title", data: []byte(durableImportTitle)},
			{name: "author", data: []byte(durableImportAuthor)},
			{name: "tocRule", data: []byte(`^第.+章.*$`)},
		}
		if staged {
			parts = append(parts, directLocalImportMultipartPart{name: "importToken", data: []byte(token)})
		} else {
			parts = append(parts, directLocalImportMultipartPart{name: "file", filename: "durable.txt", data: []byte(durableImportText)})
		}
		for _, id := range categoryIDs {
			parts = append(parts, directLocalImportMultipartPart{name: "categoryIds", data: []byte(strconv.FormatUint(uint64(id), 10))})
		}
		request, response := directLocalImportMultipartRequest(t, route.endpoint, auth, parts)
		return request, response, token
	}
	if !staged {
		root := server.cfg.LocalStoreDir
		if strings.HasPrefix(route.name, "webdav") {
			root = server.webdavDir()
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		stageLifecycleWrite(t, filepath.Join(root, "durable.txt"), []byte(durableImportText))
	}
	body, err := json.Marshal(localBookImportRequest{
		Items: []localBookImportItem{{Path: "durable.txt", ImportToken: token,
			Title: durableImportTitle, Author: durableImportAuthor, TOCRule: `^第.+章.*$`}},
		CategoryIDs: categoryIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, route.endpoint, bytes.NewReader(body))
	request.Header.Set("Authorization", auth)
	request.Header.Set("Content-Type", "application/json")
	return request, httptest.NewRecorder(), token
}

func durableImportDBSnapshot(t *testing.T, server *Server) []byte {
	t.Helper()
	var snapshot struct {
		Books      []models.Book
		Chapters   []models.Chapter
		Categories []models.BookCategory
		Progress   []models.ReadingProgress
		Bookmarks  []models.Bookmark
	}
	for _, rows := range []any{&snapshot.Books, &snapshot.Chapters, &snapshot.Categories, &snapshot.Progress, &snapshot.Bookmarks} {
		if err := server.db.Order("id asc").Find(rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func durableImportFileSnapshot(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected non-regular test fixture %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func durableImportAssertFailure(t *testing.T, response *httptest.ResponseRecorder, route stageLifecycleRoute) {
	t.Helper()
	if route.direct {
		assertDirectLocalImportError(t, response, http.StatusInternalServerError, "failed to import book")
		return
	}
	var result struct {
		Imported []struct {
			Book  *models.Book `json:"book"`
			Error string       `json:"error"`
		} `json:"imported"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(result.Imported) != 1 || result.Imported[0].Book != nil || result.Imported[0].Error != "failed to import book" {
		t.Errorf("failed durable item reported success/unsafe error: %d %s", response.Code, response.Body.String())
	}
}

// Select an actual relation value, not a number of Create calls. A future
// implementation may insert relations individually or as one batch.
func durableImportContainsCategory(value reflect.Value, id uint) bool {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		value = value.Elem()
	}
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			if durableImportContainsCategory(value.Index(i), id) {
				return true
			}
		}
		return false
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	field := value.FieldByName("CategoryID")
	return field.IsValid() && field.Kind() == reflect.Uint && uint(field.Uint()) == id
}

func TestLocalImportDurableCommitCategoryFailureRollsBackWholeItem(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, placeholder := range []bool{false, true} {
				for _, failure := range []string{"first-create", "second-create", "delete"} {
					t.Run(fmt.Sprintf("%s/token=%v/placeholder=%v/%s", route.name, staged, placeholder, failure), func(t *testing.T) {
						router, server := setupTestServer(t)
						auth := authHeader(t, router)
						ids := durableImportCategories(t, server)
						if placeholder {
							durableImportPlaceholder(t, server, ids[0])
						}
						request, response, token := durableImportRequest(t, server, route, auth, staged, ids[1:])
						before := durableImportDBSnapshot(t, server)
						filesBefore := durableImportFileSnapshot(t, server.cfg.LibraryDir)
						client := server.hub.AddClient(1, nil)
						t.Cleanup(func() { server.hub.RemoveClient(client) })
						fired := false
						const name = "test:durable-import-category-failure"
						callback := func(tx *gorm.DB) {
							if tx.Statement.Table != "book_categories" {
								return
							}
							if failure != "delete" {
								id := ids[1]
								if failure == "second-create" {
									id = ids[2]
								}
								if !durableImportContainsCategory(tx.Statement.ReflectValue, id) {
									return
								}
							}
							fired = true
							tx.AddError(errors.New("injected durable SQLite failure /private/test-owned/import-secret"))
						}
						if failure == "delete" {
							if err := server.db.Callback().Delete().Before("gorm:delete").Register(name, callback); err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = server.db.Callback().Delete().Remove(name) })
						} else {
							if err := server.db.Callback().Create().Before("gorm:create").Register(name, callback); err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name) })
						}
						router.ServeHTTP(response, request)
						if !fired {
							t.Fatal("actual category write fixture did not fire")
						}
						durableImportAssertFailure(t, response, route)
						if after := durableImportDBSnapshot(t, server); !bytes.Equal(before, after) {
							t.Errorf("failed item changed durable Book/Chapter/group/progress/bookmark state: before=%s after=%s", before, after)
						}
						if after := durableImportFileSnapshot(t, server.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
							t.Errorf("failed item left archive files: %v", after)
						}
						if events := drainBookWriteEvents(client.Send); len(events) != 0 {
							t.Errorf("failed item emitted %d success events", len(events))
						}
						if token != "" {
							if _, data, err := server.loadStagedLocalImport(1, token); err != nil || string(data) != durableImportText {
								t.Errorf("failed item consumed retry stage: %v", err)
							}
						}
					})
				}
			}
		}
	}
}

func TestLocalImportDurableCommitNormalNewItemControl(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/token=%v", route.name, staged), func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				ids := durableImportCategories(t, server)
				request, response, token := durableImportRequest(t, server, route, auth, staged, ids[1:])
				client := server.hub.AddClient(1, nil)
				t.Cleanup(func() { server.hub.RemoveClient(client) })
				router.ServeHTTP(response, request)
				want := 200
				if route.direct {
					want = 201
				}
				if response.Code != want || strings.Contains(response.Body.String(), `"error"`) {
					t.Fatalf("normal import: %d %s", response.Code, response.Body.String())
				}
				var book models.Book
				if err := server.db.First(&book).Error; err != nil {
					t.Fatal(err)
				}
				if book.CategoryID == nil || *book.CategoryID != ids[1] || book.ChapterCount != 2 {
					t.Errorf("normal Book projection: %+v", book)
				}
				var relations []uint
				if err := server.db.Model(&models.BookCategory{}).Order("id asc").Pluck("category_id", &relations).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(relations, ids[1:]) {
					t.Errorf("normal selected memberships: %v", relations)
				}
				chapterRequest := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/books/%d/chapters/0/content", book.ID), nil)
				chapterRequest.Header.Set("Authorization", auth)
				chapterResponse := httptest.NewRecorder()
				router.ServeHTTP(chapterResponse, chapterRequest)
				if chapterResponse.Code != 200 || !strings.Contains(chapterResponse.Body.String(), "owned-durable-import-paragraph") {
					t.Errorf("normal actual Reader: %d %s", chapterResponse.Code, chapterResponse.Body.String())
				}
				if events := drainBookWriteEvents(client.Send); len(events) != 1 {
					t.Errorf("normal durable event count: %d", len(events))
				}
				if token != "" {
					if _, _, err := server.loadStagedLocalImport(1, token); err == nil {
						t.Error("normal durable stage not consumed")
					}
				}
			})
		}
	}
}

func TestLocalImportDurableCommitRollbackPreservesUnknownReplacement(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, placeholder := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/token=%v/placeholder=%v", route.name, staged, placeholder), func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					ids := durableImportCategories(t, server)
					if placeholder {
						durableImportPlaceholder(t, server, ids[0])
					}
					request, response, token := durableImportRequest(t, server, route, auth, staged, nil)
					before := durableImportDBSnapshot(t, server)
					archiveRoot := filepath.Join(server.cfg.LibraryDir, "data", "testuser", engine.SafeBookFolderName(durableImportTitle, durableImportAuthor))
					fired := false
					const name = "test:durable-import-rollback-newcomer"
					callback := func(tx *gorm.DB) {
						if tx.Statement.Table != "books" || fired {
							return
						}
						fired = true
						if err := os.Rename(archiveRoot, archiveRoot+"-held"); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(archiveRoot, 0o755); err != nil {
							t.Fatal(err)
						}
						stageLifecycleWrite(t, filepath.Join(archiveRoot, "unknown-newcomer"), []byte("preserve-unknown-entity"))
						tx.AddError(errors.New("injected post-allocation failure /private/test-owned/rollback-secret"))
					}
					if placeholder {
						if err := server.db.Callback().Update().Before("gorm:update").Register(name, callback); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = server.db.Callback().Update().Remove(name) })
					} else {
						if err := server.db.Callback().Create().Before("gorm:create").Register(name, callback); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name) })
					}
					router.ServeHTTP(response, request)
					if !fired {
						t.Fatal("actual post-allocation rollback fixture did not fire")
					}
					durableImportAssertFailure(t, response, route)
					stageLifecycleAssertBytes(t, filepath.Join(archiveRoot, "unknown-newcomer"), []byte("preserve-unknown-entity"))
					stageLifecycleAssertBytes(t, filepath.Join(archiveRoot+"-held", "durable.txt"), []byte(durableImportText))
					if after := durableImportDBSnapshot(t, server); !bytes.Equal(before, after) {
						t.Errorf("rollback changed original database state: %s", after)
					}
					if token != "" {
						if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
							t.Errorf("rollback consumed original stage: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestLocalImportDurableCommitRejectsInitialLibraryAncestorSymlink(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, placeholder := range []bool{false, true} {
				for _, level := range []string{"library", "data", "user"} {
					t.Run(fmt.Sprintf("%s/token=%v/placeholder=%v/%s", route.name, staged, placeholder, level), func(t *testing.T) {
						router, server := setupTestServer(t)
						auth := authHeader(t, router)
						ids := durableImportCategories(t, server)
						if placeholder {
							durableImportPlaceholder(t, server, ids[0])
						}
						request, response, token := durableImportRequest(t, server, route, auth, staged, nil)
						before := durableImportDBSnapshot(t, server)
						owner := filepath.Join(server.cfg.LibraryDir, "data", "testuser")
						if err := os.MkdirAll(owner, 0o755); err != nil {
							t.Fatal(err)
						}
						replaced := server.cfg.LibraryDir
						if level == "data" {
							replaced = filepath.Dir(owner)
						} else if level == "user" {
							replaced = owner
						}
						foreign := t.TempDir()
						stageLifecycleWrite(t, filepath.Join(foreign, "foreign-sentinel"), []byte("never-touch-foreign-library"))
						foreignBefore := durableImportFileSnapshot(t, foreign)
						if err := os.Rename(replaced, replaced+"-held"); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(foreign, replaced); err != nil {
							t.Fatal(err)
						}
						client := server.hub.AddClient(1, nil)
						t.Cleanup(func() { server.hub.RemoveClient(client) })
						router.ServeHTTP(response, request)
						durableImportAssertFailure(t, response, route)
						if after := durableImportFileSnapshot(t, foreign); !reflect.DeepEqual(foreignBefore, after) {
							t.Errorf("initial %s symlink wrote foreign archive: %v", level, after)
						}
						if after := durableImportDBSnapshot(t, server); !bytes.Equal(before, after) {
							t.Errorf("unsafe initial archive changed database: %s", after)
						}
						if events := drainBookWriteEvents(client.Send); len(events) != 0 {
							t.Errorf("unsafe initial archive emitted %d success events", len(events))
						}
						if token != "" {
							if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
								t.Errorf("unsafe archive consumed original stage: %v", err)
							}
						}
						info, err := os.Lstat(replaced)
						if err != nil || info.Mode()&os.ModeSymlink == 0 {
							t.Errorf("unsafe ancestor repaired/removed: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestLocalImportDurableCommitCancellationDuringChapterWrite(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, placeholder := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/token=%v/placeholder=%v", route.name, staged, placeholder), func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					ids := durableImportCategories(t, server)
					if placeholder {
						durableImportPlaceholder(t, server, ids[0])
					}
					request, response, token := durableImportRequest(t, server, route, auth, staged, ids[1:])
					ctx, cancel := context.WithCancel(request.Context())
					defer cancel()
					before := durableImportDBSnapshot(t, server)
					filesBefore := durableImportFileSnapshot(t, server.cfg.LibraryDir)
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					fired := false
					const name = "test:durable-import-actual-chapter-cancel"
					if err := server.db.Callback().Create().After("gorm:create").Register(name, func(tx *gorm.DB) {
						if tx.Statement.Table == "chapters" && !fired {
							fired = true
							cancel()
						}
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name) })
					router.ServeHTTP(response, request.WithContext(ctx))
					if !fired {
						t.Fatal("actual chapter SQL cancellation fixture did not fire")
					}
					if response.Code != 500 || !strings.Contains(response.Body.String(), `"error":"local book import canceled"`) {
						t.Errorf("chapter-work cancellation reported success/wrong error: %d %s", response.Code, response.Body.String())
					}
					if after := durableImportDBSnapshot(t, server); !bytes.Equal(before, after) {
						t.Errorf("canceled item changed database: %s", after)
					}
					if after := durableImportFileSnapshot(t, server.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
						t.Errorf("canceled item left archive files: %v", after)
					}
					if events := drainBookWriteEvents(client.Send); len(events) != 0 {
						t.Errorf("canceled item emitted %d success events", len(events))
					}
					if token != "" {
						if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
							t.Errorf("canceled item lost retry stage: %v", err)
						}
					}
				})
			}
		}
	}
}
