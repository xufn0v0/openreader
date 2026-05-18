package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveImportedBookCreatesPortableLibraryFiles(t *testing.T) {
	libraryDir := t.TempDir()

	archive, err := ArchiveImportedBook(libraryDir, "yuchangsheng", "御仙1-86", "", "御仙1-86.txt", []byte("序章\n正文"))
	if err != nil {
		t.Fatalf("ArchiveImportedBook() error = %v", err)
	}

	if archive.Directory != filepath.Join("data", "yuchangsheng", "御仙1-86_") {
		t.Fatalf("Directory = %q, want %q", archive.Directory, filepath.Join("data", "yuchangsheng", "御仙1-86_"))
	}

	if _, err := os.Stat(filepath.Join(libraryDir, archive.OriginalFile)); err != nil {
		t.Fatalf("original file was not created: %v", err)
	}

	source := ArchivedBookSource{
		BookURL:            archive.OriginalFile,
		Origin:             "loc_book",
		OriginName:         archive.OriginalFile,
		Type:               0,
		Name:               "御仙1-86",
		LatestChapterTitle: "序章",
		TOCURL:             archive.TOCFile,
	}
	if err := WriteBookSource(libraryDir, archive, source); err != nil {
		t.Fatalf("WriteBookSource() error = %v", err)
	}
	if err := WriteChapterArchive(libraryDir, archive, []ArchivedChapter{{Title: "序章", BookURL: archive.OriginalFile}}); err != nil {
		t.Fatalf("WriteChapterArchive() error = %v", err)
	}

	for _, name := range []string{archive.SourceFile, archive.TOCFile} {
		if _, err := os.Stat(filepath.Join(libraryDir, name)); err != nil {
			t.Fatalf("%s was not created: %v", name, err)
		}
	}
}
