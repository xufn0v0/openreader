package localbook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

func durableWorkFixture(t *testing.T, extension string, restore bool) (Importer, ImportRequest, *models.Book) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{DataDir: filepath.Join(root, "data"), CacheDir: filepath.Join(root, "cache"),
		LibraryDir: filepath.Join(root, "library"), DatabasePath: filepath.Join(root, "data", "db.sqlite")}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	user := models.User{Username: "durable-work-user", PasswordHash: "test-only-hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{UserID: user.ID, UserName: user.Username, FileName: "original" + extension,
		Extension: extension, Title: "durable-work-book", Author: "owned-author"}
	switch extension {
	case ".epub":
		request.Data, request.TOCRule = localBookTestEPUB(t), "spin+toc"
	case ".cbz":
		request.Data = localBookTestCBZ(t)
	default:
		request.Data, request.TOCRule = []byte("第一章\nowned-first\n第二章\nowned-second"), `^第.+章$`
	}
	importer := NewImporter(cfg, database)
	if !restore {
		return importer, request, nil
	}
	book := models.Book{UserID: user.ID, Title: request.Title, Author: request.Author,
		URL: "storage/data/default/original" + extension, Intro: "preserved metadata", ChapterCount: 2}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Offset: 7, ChapterTitle: "第一章"},
		&models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterIndex: 0, Offset: 9, Title: "preserved bookmark"},
	} {
		if err := database.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return importer, request, &book
}

func durableWorkSnapshot(t *testing.T, importer Importer) []byte {
	t.Helper()
	var snapshot struct {
		Books    []models.Book
		Chapters []models.Chapter
		Groups   []models.BookCategory
		Progress []models.ReadingProgress
		Marks    []models.Bookmark
	}
	// Observe durable state independently of a canceled caller context.
	for _, rows := range []any{&snapshot.Books, &snapshot.Chapters, &snapshot.Groups, &snapshot.Progress, &snapshot.Marks} {
		if err := importer.db.WithContext(context.Background()).Order("id asc").Find(rows).Error; err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func durableWorkFiles(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular test fixture %q", path)
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

func runDurableWorkImport(importer Importer, request ImportRequest, existing *models.Book) (models.Book, error) {
	if existing != nil {
		return importer.RestoreExisting(*existing, request)
	}
	return importer.Import(request)
}

func TestImporterDurableWorkCannotReadmitReplacedArchive(t *testing.T) {
	for _, extension := range []string{".txt", ".epub", ".cbz"} {
		for _, restore := range []bool{false, true} {
			for _, phase := range []string{"raw-written", "chapter-written", "source-written", "toc-written"} {
				for _, level := range []string{"book", "user"} {
					t.Run(fmt.Sprintf("%s/restore=%v/%s/%s", extension, restore, phase, level), func(t *testing.T) {
						importer, request, existing := durableWorkFixture(t, extension, restore)
						before := durableWorkSnapshot(t, importer)
						fired := false
						var foreign, heldOriginal string
						var foreignBefore map[string][32]byte
						importer.testAfterWork = func(current, root string) {
							if fired || current != phase {
								return
							}
							fired = true
							replaced := root
							if level == "user" {
								replaced = filepath.Dir(root)
							}
							relative, err := filepath.Rel(replaced, root)
							if err != nil {
								t.Fatal(err)
							}
							foreign = filepath.Join(t.TempDir(), "replacement")
							if err := os.MkdirAll(filepath.Join(foreign, relative), 0o755); err != nil {
								t.Fatal(err)
							}
							for path, data := range map[string][]byte{
								filepath.Join(foreign, "sentinel"):                 []byte("unknown-archive-owner"),
								filepath.Join(foreign, relative, request.FileName): request.Data,
							} {
								if err := os.WriteFile(path, data, 0o644); err != nil {
									t.Fatal(err)
								}
							}
							foreignBefore = durableWorkFiles(t, foreign)
							if err := os.Rename(replaced, replaced+"-held"); err != nil {
								t.Fatal(err)
							}
							heldOriginal = filepath.Join(replaced+"-held", relative, request.FileName)
							if err := os.Rename(foreign, replaced); err != nil {
								t.Fatal(err)
							}
							foreign = replaced
						}
						book, err := runDurableWorkImport(importer, request, existing)
						if !fired {
							t.Fatal("actual archive work fixture did not fire")
						}
						if err == nil || book.ID != 0 {
							t.Errorf("later helper re-admitted replacement archive: book=%d err=%v", book.ID, err)
						}
						if after := durableWorkSnapshot(t, importer); !bytes.Equal(before, after) {
							t.Errorf("changed archive committed database state: %s", after)
						}
						if after := durableWorkFiles(t, foreign); !reflect.DeepEqual(foreignBefore, after) {
							t.Errorf("later work wrote/deleted replacement archive: %v", after)
						}
						data, readErr := os.ReadFile(heldOriginal)
						if readErr != nil || !bytes.Equal(data, request.Data) {
							t.Errorf("original held source changed: %v", readErr)
						}
					})
				}
			}
		}
	}
}

func TestImporterDurableWorkCancellationStopsBeforeNextStep(t *testing.T) {
	for _, extension := range []string{".txt", ".epub", ".cbz"} {
		for _, restore := range []bool{false, true} {
			for _, phase := range []string{"raw-written", "chapter-written", "source-written", "toc-written", "resources-prepared"} {
				if extension == ".txt" && phase == "resources-prepared" {
					continue
				}
				t.Run(fmt.Sprintf("%s/restore=%v/%s", extension, restore, phase), func(t *testing.T) {
					importer, request, existing := durableWorkFixture(t, extension, restore)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					importer.db = importer.db.WithContext(ctx)
					before := durableWorkSnapshot(t, importer)
					filesBefore := durableWorkFiles(t, importer.cfg.LibraryDir)
					fired := false
					var continued []string
					importer.testAfterWork = func(current, _ string) {
						if fired {
							continued = append(continued, current)
						} else if current == phase {
							fired = true
							cancel()
						}
					}
					_, err := runDurableWorkImport(importer, request, existing)
					if !fired {
						t.Fatal("actual archive work cancellation fixture did not fire")
					}
					if !errors.Is(err, context.Canceled) {
						t.Errorf("actual work cancellation was swallowed: %v", err)
					}
					if len(continued) != 0 {
						t.Errorf("continued actual archive work after cancellation: %v", continued)
					}
					if after := durableWorkSnapshot(t, importer); !bytes.Equal(before, after) {
						t.Errorf("cancelled work committed rows: %s", after)
					}
					if after := durableWorkFiles(t, importer.cfg.LibraryDir); !reflect.DeepEqual(filesBefore, after) {
						t.Errorf("cancelled work left owned archive files: %v", after)
					}
				})
			}
		}
	}
}

func TestImporterDurableWorkRollbackRetainsUnknownMember(t *testing.T) {
	for _, extension := range []string{".txt", ".epub", ".cbz"} {
		for _, restore := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/restore=%v", extension, restore), func(t *testing.T) {
				importer, request, existing := durableWorkFixture(t, extension, restore)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				importer.db = importer.db.WithContext(ctx)
				fired := false
				var unknown string
				importer.testAfterWork = func(phase, root string) {
					if fired || phase != "raw-written" {
						return
					}
					fired = true
					unknown = filepath.Join(root, "unknown-member")
					if err := os.WriteFile(unknown, []byte("retain-unknown-member"), 0o644); err != nil {
						t.Fatal(err)
					}
					cancel()
				}
				_, err := runDurableWorkImport(importer, request, existing)
				if !fired || err == nil {
					t.Fatalf("actual cancellation/rollback fixture not observed: %v %v", fired, err)
				}
				data, err := os.ReadFile(unknown)
				if err != nil || string(data) != "retain-unknown-member" {
					t.Errorf("rollback removed an unknown archive member: %q %v", data, err)
				}
			})
		}
	}
}
