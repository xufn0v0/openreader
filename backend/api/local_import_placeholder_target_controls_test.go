package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"

	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// These are post-implementation coverage controls, not additional pre-b4 reds.
// The mutation commits on an independent SQLite pool after the actual placeholder
// count has returned, before the importer opens its item transaction.
func TestLocalImportPlaceholderTargetChangedBeforeTransaction(t *testing.T) {
	for _, route := range durableImportConfirmRoutes() {
		for _, staged := range []bool{false, true} {
			for _, mutation := range []string{"delete", "owner", "source", "url", "library", "original"} {
				t.Run(fmt.Sprintf("%s/token=%v/%s", route.name, staged, mutation), func(t *testing.T) {
					router, server := setupTestServer(t)
					auth := authHeader(t, router)
					ids := durableImportCategories(t, server)
					book := durableImportPlaceholder(t, server, ids[0])
					request, response, token := durableImportRequest(t, server, route, auth, staged, ids[1:])
					external, err := readerdb.Open(server.cfg)
					if err != nil {
						t.Fatal(err)
					}
					sqlDB, err := external.DB()
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = sqlDB.Close() })
					foreign := models.User{Username: "target-control-foreign", PasswordHash: "test-only-hash"}
					source := models.BookSource{Name: "target-control-source", BaseURL: "https://example.invalid"}
					for _, row := range []any{&foreign, &source} {
						if err := external.Create(row).Error; err != nil {
							t.Fatal(err)
						}
					}
					bait := filepath.Join(server.cfg.LibraryDir, "target-control-bait")
					if err := os.MkdirAll(bait, 0o755); err != nil {
						t.Fatal(err)
					}
					stageLifecycleWrite(t, filepath.Join(bait, "original.txt"), []byte("existing-other-archive"))
					filesBefore := durableImportFileSnapshot(t, server.cfg.LibraryDir)
					client := server.hub.AddClient(1, nil)
					t.Cleanup(func() { server.hub.RemoveClient(client) })
					var afterExternal []byte
					fired := false
					const callback = "test:import-placeholder-pretransaction-target"
					if err := server.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
						if fired || tx.Error != nil || tx.Statement.Table != "chapters" {
							return
						}
						if _, count := tx.Statement.Dest.(*int64); !count {
							return
						}
						if _, inTransaction := tx.Statement.ConnPool.(interface {
							Commit() error
							Rollback() error
						}); inTransaction {
							t.Fatal("fixture must precede item transaction")
						}
						fired = true
						statement, args := "DELETE FROM books WHERE id = ? AND user_id = ?", []any{book.ID, 1}
						if mutation != "delete" {
							column, value := "url", any("local://externally-rebound")
							switch mutation {
							case "owner":
								column, value = "user_id", foreign.ID
							case "source":
								column, value = "source_id", source.ID
							case "library":
								column, value = "library_path", "target-control-bait"
							case "original":
								column, value = "original_file", "target-control-bait/original.txt"
							}
							statement, args = "UPDATE books SET "+column+" = ? WHERE id = ? AND user_id = ?", []any{value, book.ID, 1}
						}
						changed := external.Exec(statement, args...)
						if changed.Error != nil || changed.RowsAffected != 1 {
							t.Fatalf("external target mutation did not commit: %d %v", changed.RowsAffected, changed.Error)
						}
						afterExternal = durableImportDBSnapshot(t, server)
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = server.db.Callback().Query().Remove(callback) })
					router.ServeHTTP(response, request)
					if !fired || len(afterExternal) == 0 {
						t.Fatal("actual pre-transaction target fixture did not fire")
					}
					durableImportAssertFailure(t, response, route)
					if after := durableImportDBSnapshot(t, server); !bytes.Equal(afterExternal, after) {
						t.Errorf("rejected import changed external target result: %s", after)
					}
					if after := durableImportFileSnapshot(t, server.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
						t.Errorf("rejected import left files or touched existing archive: %v", after)
					}
					if events := drainBookWriteEvents(client.Send); len(events) != 0 {
						t.Errorf("rejected import emitted %d success events", len(events))
					}
					if token != "" {
						if _, _, err := server.loadStagedLocalImport(1, token); err != nil {
							t.Errorf("rejected import consumed retry stage: %v", err)
						}
					}
				})
			}
		}
	}
}
