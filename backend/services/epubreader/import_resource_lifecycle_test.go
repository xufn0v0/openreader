package epubreader

import (
	"archive/zip"
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func importResourceFixture(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, name := range []string{"001.png", "002.png"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("owned-" + name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestImportResourceExtractionRetainsOriginalDestination(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "unknown-member"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			source, destination := filepath.Join(parent, "source.epub"), filepath.Join(parent, "extract")
			if err := os.WriteFile(source, importResourceFixture(t), 0o644); err != nil {
				t.Fatal(err)
			}
			fired := false
			foreign := ""
			afterArchiveEntryTestHook = func(root, name string) {
				if fired {
					return
				}
				fired = true
				if name != "001.png" {
					t.Fatalf("unexpected first actual write %q", name)
				}
				if kind == "unknown-member" {
					foreign = destination
					if err := os.WriteFile(filepath.Join(root, "002.png"), []byte("unknown-member"), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				if err := os.Rename(root, root+"-held"); err != nil {
					t.Fatal(err)
				}
				foreign = filepath.Join(parent, "foreign")
				if err := os.Mkdir(foreign, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(foreign, "sentinel"), []byte("unknown-target"), 0o644); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					if err := os.Symlink(foreign, root); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Rename(foreign, root); err != nil {
						t.Fatal(err)
					}
					foreign = root
				}
			}
			t.Cleanup(func() { afterArchiveEntryTestHook = nil })
			err := extractArchiveFile(source, destination, defaultExtractionLimits())
			if !fired {
				t.Fatal("actual resource entry write fixture did not fire")
			}
			if err == nil {
				t.Error("changed/unknown extraction destination accepted")
			}
			if kind == "unknown-member" {
				data, err := os.ReadFile(filepath.Join(foreign, "002.png"))
				if err != nil || string(data) != "unknown-member" {
					t.Errorf("failed extraction removed unknown member: %q %v", data, err)
				}
			} else {
				entries, err := os.ReadDir(foreign)
				if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
					t.Errorf("resource copy entered replacement: %v %v", entries, err)
				}
				data, err := os.ReadFile(filepath.Join(destination+"-held", "001.png"))
				if err != nil || string(data) != "owned-001.png" {
					t.Errorf("original owned resource changed: %q %v", data, err)
				}
			}
		})
	}
}

type importResourceReader func([]byte) (int, error)

func (read importResourceReader) Read(buffer []byte) (int, error) { return read(buffer) }

func TestImportResourcePartialFailureCannotDeleteNewcomer(t *testing.T) {
	target := filepath.Join(t.TempDir(), "entry.png")
	fired := false
	reader := importResourceReader(func(buffer []byte) (int, error) {
		fired = true
		if err := os.Rename(target, target+"-held"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("unknown-partial-newcomer"), 0o644); err != nil {
			t.Fatal(err)
		}
		return copy(buffer, "partial"), io.ErrUnexpectedEOF
	})
	if err := writeBoundedFile(target, reader, 100); err == nil || !fired {
		t.Fatalf("actual partial reader failure not observed: %v %v", fired, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unknown-partial-newcomer" {
		t.Errorf("partial writer removed unknown newcomer: %q %v", data, err)
	}
}

func TestImportResourceMaxInt64BudgetDoesNotDiscardBytes(t *testing.T) {
	target := filepath.Join(t.TempDir(), "entry.png")
	if err := writeBoundedFile(target, bytes.NewBufferString("bounded-original"), math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "bounded-original" {
		t.Errorf("MaxInt64 budget silently discarded bytes: %q %v", data, err)
	}
}
