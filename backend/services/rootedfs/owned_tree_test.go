package rootedfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func ownedTestTree(t *testing.T, ctx context.Context) *OwnedTree {
	t.Helper()
	tree, err := OpenOwnedTree(ctx, t.TempDir(), "data/alice")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tree.Close() })
	if err := tree.CreateRoot("book"); err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestOwnedTreeActualChunkCancellationCompensatesOnlyOwned(t *testing.T) {
	for _, relative := range []string{"original.txt", "content/aa/chapter", "bookSource.json", ".epub-resources/hash/OPS/one.xhtml", ".cbz-resources/hash/pages/001.png"} {
		for _, phase := range []string{"read", "write"} {
			for _, newcomer := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/newcomer=%v", relative, phase, newcomer), func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					tree := ownedTestTree(t, ctx)
					path, fired := filepath.Join(tree.PathLabel(), relative), false
					ownedTreeIOTestHook = func(current, label string, count int) {
						if fired || current != phase || count == 0 || label != path {
							return
						}
						fired = true
						if newcomer {
							if err := os.Rename(path, path+"-held"); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(path, []byte("unknown-chunk-newcomer"), 0o640); err != nil {
								t.Fatal(err)
							}
						}
						cancel()
					}
					t.Cleanup(func() { ownedTreeIOTestHook = nil })
					_, err := tree.Write(relative, bytes.NewReader(bytes.Repeat([]byte("x"), 96<<10)), math.MaxInt64)
					if !fired || !errors.Is(err, context.Canceled) {
						t.Fatalf("actual chunk cancellation: %v %v", fired, err)
					}
					_ = tree.Close()
					if newcomer {
						privateTestBytes(t, path, "unknown-chunk-newcomer")
						info, err := os.Stat(path + "-held")
						want := int64(0)
						if phase == "write" {
							want = 32 << 10
						}
						if err != nil || info.Size() != want {
							t.Errorf("write continued past first actual chunk: %v %v", info, err)
						}
					} else if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("owned partial retained: %v", err)
					}
				})
			}
		}
	}
}

func TestOwnedTreeOriginalSourceReadCancellationAndIdentity(t *testing.T) {
	for _, phase := range []string{"source-read", "source-read-at"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tree := ownedTestTree(t, ctx)
			if _, err := tree.Write("original", bytes.NewBufferString("original-bytes"), 14); err != nil {
				t.Fatal(err)
			}
			reader, err := tree.Open("original")
			if err != nil {
				t.Fatal(err)
			}
			fired := false
			ownedTreeIOTestHook = func(current, _ string, count int) {
				if current == phase && count > 0 {
					fired = true
					cancel()
				}
			}
			t.Cleanup(func() { ownedTreeIOTestHook = nil })
			buffer := make([]byte, 8)
			if phase == "source-read" {
				_, err = reader.Read(buffer)
			} else {
				_, err = reader.ReadAt(buffer, 0)
			}
			_ = reader.Close()
			if !fired || !errors.Is(err, context.Canceled) {
				t.Fatalf("actual source cancellation: %v %v", fired, err)
			}
			_ = tree.Close()
		})
	}
}

func TestOwnedTreeActualPartialDirectoryCancellationRetainsUnknown(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tree, err := OpenOwnedTree(ctx, t.TempDir(), "data/alice")
			if err != nil {
				t.Fatal(err)
			}
			fired, temporary := false, ""
			ownedTreeDirectoryCreatedTestHook = func(path string) {
				fired, temporary = true, path
				if unknown {
					if err := os.WriteFile(filepath.Join(path, "unknown"), []byte("retain"), 0o640); err != nil {
						t.Fatal(err)
					}
				}
				cancel()
			}
			t.Cleanup(func() { ownedTreeDirectoryCreatedTestHook = nil })
			err = tree.CreateRoot("book")
			_ = tree.Close()
			if !fired || !errors.Is(err, context.Canceled) {
				t.Fatalf("partial mkdir fixture: %v %v", fired, err)
			}
			if unknown {
				privateTestBytes(t, filepath.Join(temporary, "unknown"), "retain")
			} else if _, err := os.Lstat(temporary); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("owned empty temporary retained: %v", err)
			}
		})
	}
}

func TestOwnedTreeBudgetsModesAndHandleOwnership(t *testing.T) {
	for _, limit := range []int64{3, 4, math.MaxInt64} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			tree := ownedTestTree(t, context.Background())
			_, err := tree.Write("source", bytes.NewBufferString("four"), limit)
			if limit < 4 {
				if !errors.Is(err, ErrWriteLimit) {
					t.Fatalf("overflow: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := tree.ChmodOwned("source", 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(tree.PathLabel(), "source"))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("owned marker mode: %v %v", info, err)
			}
		})
	}
	root := t.TempDir()
	countOpen := func() int {
		count := 0
		for fd := 0; fd < 4096; fd++ {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
				count++
			}
		}
		return count
	}
	before := countOpen()
	for i := 0; i < 120; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		tree, err := OpenOwnedTree(ctx, root, "data/alice")
		if err != nil {
			t.Fatal(err)
		}
		if err := tree.CreateRoot(fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
		_, err = tree.Write("a/b/source", bytes.NewBufferString("original"), 8)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := tree.Open("a/b/source")
		if err != nil {
			t.Fatal(err)
		}
		if data, err := io.ReadAll(reader); err != nil || string(data) != "original" {
			t.Fatal(err)
		}
		_ = reader.Close()
		var descriptors []int
		for _, scope := range tree.scopes {
			descriptors = append(descriptors, int(scope.parent.file.Fd()))
		}
		if i%2 == 0 {
			tree.Keep()
		} else {
			cancel()
		}
		if err := tree.Close(); err != nil {
			t.Fatal(err)
		}
		cancel()
		for _, fd := range descriptors {
			if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
				t.Fatalf("owned directory fd remains open: %d %v", fd, err)
			}
		}
	}
	after := countOpen()
	if after > before+2 {
		t.Fatalf("owned FD leak: before=%d after=%d", before, after)
	}
}
