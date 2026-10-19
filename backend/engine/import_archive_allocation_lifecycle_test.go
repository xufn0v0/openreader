package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportArchiveAllocationCannotOwnConcurrentDirectory(t *testing.T) {
	library := t.TempDir()
	fired := false
	var newcomer string
	archiveDirectorySelectedTestHook = func(parent, name string) {
		if fired {
			return
		}
		fired = true
		newcomer = filepath.Join(parent, name, "original.txt")
		if err := os.MkdirAll(filepath.Dir(newcomer), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(newcomer, []byte("never-overwrite-concurrent-owner"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { archiveDirectorySelectedTestHook = nil })
	archive, err := ArchiveImportedBook(library, "allocation-user", "same-title", "author", "original.txt", []byte("new-import-bytes"))
	if !fired {
		t.Fatal("actual selected-name collision fixture did not fire")
	}
	data, readErr := os.ReadFile(newcomer)
	if readErr != nil || string(data) != "never-overwrite-concurrent-owner" {
		t.Errorf("allocation overwrote another owner's original: %q %v", data, readErr)
	}
	if err == nil && filepath.Join(library, archive.OriginalFile) == newcomer {
		t.Error("allocation claimed a directory it did not create")
	}
}

func TestImportArchiveAllocationFailureCompensatesOwnedEmptyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires nonroot native permission enforcement")
	}
	library := t.TempDir()
	bookRoot := filepath.Join(library, "data", "allocation-user", SafeBookFolderName("owned-title", "author"))
	fired := false
	archiveDirectoryCreatedTestHook = func(directory string) {
		fired = true
		if directory != bookRoot {
			t.Fatal("unexpected owned directory")
		}
		if err := os.Chmod(directory, 0); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		archiveDirectoryCreatedTestHook = nil
		_ = os.Chmod(bookRoot, 0o755)
	})
	_, err := ArchiveImportedBook(library, "allocation-user", "owned-title", "author", "original.txt", []byte("original"))
	if !fired || !os.IsPermission(err) {
		t.Fatalf("actual original-write permission failure not observed: %v %v", fired, err)
	}
	if _, err := os.Lstat(bookRoot); !os.IsNotExist(err) {
		t.Errorf("failed initial original write left its own empty archive: %v", err)
	}
}
