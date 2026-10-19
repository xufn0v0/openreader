package localbook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// The independent pool commits at the observed real raw-file write, before the
// item transaction. These supplement b4 coverage; no application guard is removed
// to manufacture a red, and an in-transaction fault is not called external work.
func TestImporterPlaceholderTargetChangedBeforeTransaction(t *testing.T) {
	for _, extension := range []string{".txt", ".epub", ".cbz"} {
		for _, explicitRestore := range []bool{false, true} {
			for _, mutation := range []string{"delete", "owner", "source", "url", "library", "original"} {
				t.Run(fmt.Sprintf("%s/explicit=%v/%s", extension, explicitRestore, mutation), func(t *testing.T) {
					importer, request, existing := durableWorkFixture(t, extension, true)
					external, err := readerdb.Open(importer.cfg)
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
					bait := filepath.Join(importer.cfg.LibraryDir, "target-control-bait")
					if err := os.MkdirAll(bait, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(bait, "original.txt"), []byte("existing-other-archive"), 0o644); err != nil {
						t.Fatal(err)
					}
					filesBefore := durableWorkFiles(t, importer.cfg.LibraryDir)
					var afterExternal []byte
					fired := false
					importer.testAfterWork = func(phase, _ string) {
						if fired || phase != "raw-written" {
							return
						}
						fired = true
						statement, args := "DELETE FROM books WHERE id = ? AND user_id = ?", []any{existing.ID, request.UserID}
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
							statement, args = "UPDATE books SET "+column+" = ? WHERE id = ? AND user_id = ?", []any{value, existing.ID, request.UserID}
						}
						changed := external.Exec(statement, args...)
						if changed.Error != nil || changed.RowsAffected != 1 {
							t.Fatalf("external target mutation did not commit: %d %v", changed.RowsAffected, changed.Error)
						}
						afterExternal = durableWorkSnapshot(t, importer)
					}
					var book models.Book
					if explicitRestore {
						book, err = importer.RestoreExisting(*existing, request)
					} else {
						book, err = importer.Import(request)
					}
					if !fired || len(afterExternal) == 0 {
						t.Fatal("actual pre-transaction target fixture did not fire")
					}
					if err == nil || book.ID != 0 {
						t.Errorf("changed target accepted or resurrected: book=%d err=%v", book.ID, err)
					}
					if after := durableWorkSnapshot(t, importer); !bytes.Equal(afterExternal, after) {
						t.Errorf("rejected import changed external target result: %s", after)
					}
					if after := durableWorkFiles(t, importer.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
						t.Errorf("rejected import left files or touched existing archive: %v", after)
					}
				})
			}
		}
	}
}
