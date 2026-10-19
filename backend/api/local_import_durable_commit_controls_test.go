package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/models"
)

// These exercise the new transaction boundary. They are not additional
// old-implementation reds (the old implementation had no final Book reload).
func TestLocalImportDurableCommitLateDatabaseFailureControls(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, placeholder := range []bool{false, true} {
				for _, phase := range []string{"category-recheck", "final-book-reload"} {
					t.Run(fmt.Sprintf("%s/token=%v/placeholder=%v/%s", route.name, staged, placeholder, phase), func(t *testing.T) {
						router, server := setupTestServer(t)
						auth := authHeader(t, router)
						ids := durableImportCategories(t, server)
						if placeholder {
							durableImportPlaceholder(t, server, ids[0])
						}
						request, response, token := durableImportRequest(t, server, route, auth, staged, ids[1:])
						before, filesBefore := durableImportDBSnapshot(t, server), durableImportFileSnapshot(t, server.cfg.LibraryDir)
						client := server.hub.AddClient(1, nil)
						t.Cleanup(func() { server.hub.RemoveClient(client) })
						fired, relationsWritten := false, false
						const name = "test:durable-import-late-control"
						if err := server.db.Callback().Create().After("gorm:create").Register(name, func(tx *gorm.DB) {
							if tx.Error == nil && tx.Statement.Table == "book_categories" && durableImportContainsCategory(tx.Statement.ReflectValue, ids[2]) {
								relationsWritten = true
							}
						}); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name) })
						if err := server.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
							_, inTransaction := tx.Statement.ConnPool.(interface {
								Commit() error
								Rollback() error
							})
							if !inTransaction {
								return
							}
							if phase == "category-recheck" && tx.Statement.Table == "categories" || phase == "final-book-reload" && relationsWritten && tx.Statement.Table == "books" {
								fired = true
								tx.AddError(errors.New("injected durable database failure /private/owned-control"))
							}
						}); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = server.db.Callback().Query().Remove(name) })
						router.ServeHTTP(response, request)
						if !fired {
							t.Fatal("actual late transaction failure fixture did not fire")
						}
						// Remove the fixture before independent durable-state observations.
						if err := server.db.Callback().Query().Remove(name); err != nil {
							t.Fatal(err)
						}
						durableImportAssertFailure(t, response, route)
						if after := durableImportDBSnapshot(t, server); !bytes.Equal(before, after) {
							t.Errorf("late failure changed durable rows: %s", after)
						}
						if after := durableImportFileSnapshot(t, server.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
							t.Errorf("late failure left archive files: %v", after)
						}
						if events := drainBookWriteEvents(client.Send); len(events) != 0 {
							t.Errorf("late failure emitted %d success events", len(events))
						}
						if token != "" {
							if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
								t.Error(err)
							}
						}
					})
				}
			}
		}
	}
}

func TestLocalImportDurableCommitPlaceholderMergesUnownedColumns(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/token=%v", route.name, staged), func(t *testing.T) {
				router, server := setupTestServer(t)
				auth := authHeader(t, router)
				ids := durableImportCategories(t, server)
				book := durableImportPlaceholder(t, server, ids[0])
				request, response, _ := durableImportRequest(t, server, route, auth, staged, ids[1:])
				fired := false
				const name = "test:durable-import-unowned-columns"
				if err := server.db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
					if fired || tx.Statement.Table != "books" {
						return
					}
					fired = true
					if err := tx.Exec("UPDATE books SET intro = ?, can_update = ? WHERE id = ? AND user_id = ?", "concurrent-owned-metadata", false, book.ID, 1).Error; err != nil {
						t.Fatal(err)
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = server.db.Callback().Update().Remove(name) })
				client := server.hub.AddClient(1, nil)
				t.Cleanup(func() { server.hub.RemoveClient(client) })
				router.ServeHTTP(response, request)
				want := 200
				if route.direct {
					want = 201
				}
				if !fired || response.Code != want {
					t.Fatalf("column merge control: %v %d %s", fired, response.Code, response.Body.String())
				}
				var current models.Book
				if err := server.db.First(&current, book.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.Intro != "concurrent-owned-metadata" || current.CanUpdate || current.CategoryID == nil || *current.CategoryID != ids[1] {
					t.Errorf("unowned columns or primary lost: %+v", current)
				}
				if !strings.Contains(response.Body.String(), `"intro":"concurrent-owned-metadata"`) || !strings.Contains(response.Body.String(), `"canUpdate":false`) {
					t.Errorf("response was not authoritative: %s", response.Body.String())
				}
				events := drainBookWriteEvents(client.Send)
				if len(events) != 1 || !strings.Contains(string(events[0]), `"intro":"concurrent-owned-metadata"`) || !strings.Contains(string(events[0]), `"canUpdate":false`) {
					t.Errorf("event was not authoritative: %s", events)
				}
			})
		}
	}
}

func TestLocalImportDurableCommitLaterItemFailureKeepsEarlierSuccess(t *testing.T) {
	for _, source := range []string{"local-store", "webdav"} {
		for _, staged := range []bool{false, true} {
			for _, canceled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/token=%v/cancel=%v", source, staged, canceled), func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					ids := durableImportCategories(t, server)
					items := []localBookImportItem{}
					for _, title := range []string{"first", "second"} {
						item := localBookImportItem{Path: title + ".txt", Title: title, TOCRule: `^第.+章.*$`}
						if staged {
							var err error
							item.ImportToken, err = server.stageLocalImport(1, item.Path, ".txt", []byte(durableImportText))
							if err != nil {
								t.Fatal(err)
							}
						} else {
							root := server.cfg.LocalStoreDir
							if source == "webdav" {
								root = server.webdavDir()
							}
							if err := os.MkdirAll(root, 0o755); err != nil {
								t.Fatal(err)
							}
							stageLifecycleWrite(t, filepath.Join(root, item.Path), []byte(durableImportText))
						}
						items = append(items, item)
					}
					body, err := json.Marshal(localBookImportRequest{Items: items, CategoryIDs: ids[1:]})
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					fired, booksCreated := false, 0
					const name = "test:durable-import-later-item"
					if err := server.db.Callback().Create().After("gorm:create").Register(name+":books", func(tx *gorm.DB) {
						if tx.Error == nil && tx.Statement.Table == "books" {
							booksCreated++
						}
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name + ":books") })
					if err := server.db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
						if booksCreated == 2 && tx.Statement.Table == "book_categories" && !fired {
							fired = true
							if canceled {
								cancel()
							} else {
								tx.AddError(errors.New("injected later item failure"))
							}
						}
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = server.db.Callback().Create().Remove(name) })
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					request := httptest.NewRequest(http.MethodPost, "/api/"+source+"/import", bytes.NewReader(body)).WithContext(ctx)
					request.Header.Set("Authorization", auth)
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					want := 200
					if canceled {
						want = 500
					}
					if !fired || response.Code != want {
						t.Fatalf("later failure: %v %d %s", fired, response.Code, response.Body.String())
					}
					var books []models.Book
					if err := server.db.Find(&books).Error; err != nil || len(books) != 1 || books[0].Title != "first" {
						t.Fatalf("earlier durable book lost or failed item retained: %+v %v", books, err)
					}
					var relations []uint
					if err := server.db.Model(&models.BookCategory{}).Order("id asc").Pluck("category_id", &relations).Error; err != nil || !reflect.DeepEqual(relations, ids[1:]) {
						t.Errorf("earlier durable memberships: %v %v", relations, err)
					}
					events := drainBookWriteEvents(client.Send)
					if len(events) != 1 || !strings.Contains(string(events[0]), `"title":"first"`) || strings.Contains(string(events[0]), `"title":"second"`) {
						t.Errorf("durable-only earlier event: %s", events)
					}
					if staged {
						if _, _, err := server.loadStagedLocalImport(1, items[0].ImportToken); err == nil {
							t.Error("earlier durable token retained")
						}
						if _, _, err := server.loadStagedLocalImport(1, items[1].ImportToken); err != nil {
							t.Errorf("later failed token consumed: %v", err)
						}
					}
				})
			}
		}
	}
}
