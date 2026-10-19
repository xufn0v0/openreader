package rootedfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var ErrWriteLimit = errors.New("owned file exceeds write limit")

// OwnedTree is one sequential, caller-context write session. Only files and
// directories actually created by this session are compensation candidates.
// Existing configuration ancestors are admitted once; child names never adopt
// an unknown directory. Keep is a post-durability handoff, not a new admission.
type OwnedTree struct {
	ctx     context.Context
	root    *PrivateScope
	scopes  []*PrivateScope
	dirs    map[string]*PrivateScope
	files   map[string]*PrivateEntry
	created []*PrivateEntry
	keep    bool
}

// Observes actual bounded I/O only in native tests; nil in production.
var ownedTreeIOTestHook func(phase, path string, count int)
var ownedTreeDirectoryCreatedTestHook func(path string)

func OpenOwnedTree(ctx context.Context, boundary, relative string) (*OwnedTree, error) {
	scope, err := openFileScope(ctx, boundary, relative, true, 0o755, 0o644)
	if err != nil {
		return nil, err
	}
	return &OwnedTree{ctx: ctx, root: scope, scopes: []*PrivateScope{scope},
		dirs: map[string]*PrivateScope{".": scope}, files: map[string]*PrivateEntry{}}, nil
}

func (t *OwnedTree) PathLabel() string        { return t.root.PathLabel() }
func (t *OwnedTree) Context() context.Context { return t.ctx }

func (s *PrivateScope) createExclusiveDirectory(name string) (*PrivateScope, error) {
	if !privateName(name) {
		return nil, ErrUnsafePath
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.parent.meta.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return nil, os.ErrExist
		}
		return nil, err
	}
	temporary, err := randomName(".openreader-directory-", 12)
	if err != nil {
		return nil, err
	}
	if err := unix.Mkdirat(int(s.parent.file.Fd()), temporary, uint32(s.directoryMode.Perm())); err != nil {
		return nil, err
	}
	stat, err := statWriteName(s.parent.file, temporary)
	if err != nil {
		return nil, err
	}
	owned := &createdDirectory{parent: s.parent.file, name: temporary, stat: stat}
	complete := false
	defer func() {
		if !complete {
			removeCreatedEmptyDirectory(owned)
		}
	}()
	info, err := s.parent.meta.Lstat(temporary)
	if err != nil {
		return nil, err
	}
	opened, err := openReadDirectory(s.parent, temporary, info)
	if err != nil {
		return nil, err
	}
	defer closeReadDirectory(opened)
	if ownedTreeDirectoryCreatedTestHook != nil {
		ownedTreeDirectoryCreatedTestHook(filepath.Join(s.label, temporary))
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if err := validateCreatedDirectory(owned); err != nil {
		return nil, err
	}
	if err := renameWriteNoReplace(s.parent.file, temporary, name); err != nil {
		if errors.Is(err, unix.EEXIST) || errors.Is(err, unix.ENOTEMPTY) {
			return nil, os.ErrExist
		}
		return nil, writePublicationError(err)
	}
	owned.name = name
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if err := validateCreatedDirectory(owned); err != nil {
		return nil, err
	}
	handle := s.read.anchor.retainEntry(s.parent, name, name, info)
	read, err := NewReadScope(handle)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	child := &PrivateScope{read: read, parent: read.directory, ctx: s.ctx, label: filepath.Join(s.label, name),
		created: []*createdDirectory{owned}, directoryMode: s.directoryMode, fileMode: s.fileMode}
	complete = true
	return child, nil
}

// CreateRoot exclusively selects a new child as the book root. A collision
// never transfers ownership; callers may select another friendly suffix.
func (t *OwnedTree) CreateRoot(name string) error {
	if len(t.files) != 0 || len(t.dirs) != 1 {
		return ErrUnsafePath
	}
	child, err := t.root.createExclusiveDirectory(name)
	if err != nil {
		return err
	}
	t.scopes = append(t.scopes, child)
	t.root = child
	t.dirs = map[string]*PrivateScope{".": child}
	return nil
}

func ownedRelative(relative string) (string, error) {
	if relative == "." || relative == "" {
		return ".", nil
	}
	return cleanRelative(relative)
}

func (t *OwnedTree) directory(relative string) (*PrivateScope, error) {
	clean, err := ownedRelative(relative)
	if err != nil || t.root == nil {
		return nil, ErrUnsafePath
	}
	if err := t.root.Validate(); err != nil {
		return nil, err
	}
	if scope := t.dirs[clean]; scope != nil {
		return scope, scope.Validate()
	}
	parent, path := t.root, "."
	for _, name := range strings.Split(filepath.ToSlash(clean), "/") {
		path = filepath.ToSlash(filepath.Join(path, name))
		child := t.dirs[path]
		if child == nil {
			child, err = parent.createExclusiveDirectory(name)
			if err != nil {
				return nil, err
			}
			t.dirs[path], t.scopes = child, append(t.scopes, child)
		}
		if err := child.Validate(); err != nil {
			return nil, err
		}
		parent = child
	}
	return parent, nil
}

func (t *OwnedTree) Mkdir(relative string) error {
	_, err := t.directory(relative)
	return err
}

func (t *OwnedTree) Check() error {
	if t.root == nil {
		return ErrUnsafePath
	}
	return t.root.Validate()
}

func (t *OwnedTree) Validate() error {
	if t.root == nil {
		return ErrUnsafePath
	}
	for _, scope := range t.scopes {
		if err := scope.Validate(); err != nil {
			return err
		}
	}
	for _, entry := range t.created {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (t *OwnedTree) Write(relative string, source io.Reader, maxBytes int64) (int64, error) {
	return t.write(relative, source, maxBytes, false)
}

// ReplaceOwned is reserved for a derived marker already created by this same
// tree. It never overwrites an unadmitted or replaced target.
func (t *OwnedTree) ReplaceOwned(relative string, source io.Reader, maxBytes int64) (int64, error) {
	return t.write(relative, source, maxBytes, true)
}

func (t *OwnedTree) write(relative string, source io.Reader, maxBytes int64, replace bool) (written int64, err error) {
	if maxBytes < 0 {
		return 0, ErrWriteLimit
	}
	clean, err := cleanRelative(relative)
	if err != nil {
		return 0, err
	}
	parent, err := t.directory(filepath.Dir(clean))
	if err != nil {
		return 0, err
	}
	entry := t.files[clean]
	if entry != nil && !replace {
		return 0, ErrUnsafePath
	}
	if entry == nil {
		entry, err = parent.Snapshot(filepath.Base(clean))
		if err != nil {
			return 0, err
		}
		if entry.Info != nil {
			return 0, ErrUnsafePath
		}
		t.files[clean], t.created = entry, append(t.created, entry)
	}
	write := func(output io.Writer) error {
		buffer := make([]byte, 32<<10)
		for {
			if err := entry.Validate(); err != nil {
				return err
			}
			remaining := maxBytes - written
			readBuffer := buffer
			if remaining < int64(len(buffer)) {
				readBuffer = buffer[:int(remaining)+1]
			}
			n, readErr := source.Read(readBuffer)
			if n < 0 || n > len(readBuffer) {
				return io.ErrUnexpectedEOF
			}
			if ownedTreeIOTestHook != nil {
				ownedTreeIOTestHook("read", filepath.Join(t.PathLabel(), clean), n)
			}
			if err := entry.Validate(); err != nil {
				return err
			}
			if int64(n) > remaining {
				return ErrWriteLimit
			}
			if n > 0 {
				count, writeErr := output.Write(readBuffer[:n])
				written += int64(count)
				if ownedTreeIOTestHook != nil {
					ownedTreeIOTestHook("write", filepath.Join(t.PathLabel(), clean), count)
				}
				if err := entry.Validate(); err != nil {
					return err
				}
				if writeErr != nil {
					return writeErr
				}
				if count != n {
					return io.ErrShortWrite
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
			if n == 0 {
				return io.ErrNoProgress
			}
		}
	}
	if entry.Info != nil {
		err = parent.Replace(entry, ".owned-marker-", write, nil)
	} else {
		err = parent.WriteNew(entry, write)
	}
	return written, err
}

type OwnedReader struct {
	file  *os.File
	entry *PrivateEntry
	info  os.FileInfo
}

func (t *OwnedTree) Open(relative string) (*OwnedReader, error) {
	clean, err := cleanRelative(relative)
	if err != nil {
		return nil, err
	}
	entry := t.files[clean]
	if entry == nil || entry.Info == nil {
		return nil, ErrUnsafePath
	}
	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(entry.Info, info) {
		_ = file.Close()
		return nil, ErrUnsafePath
	}
	return &OwnedReader{file: file, entry: entry, info: info}, nil
}

func (r *OwnedReader) Info() os.FileInfo { return r.info }
func (r *OwnedReader) Close() error      { return r.file.Close() }
func (r *OwnedReader) Read(buffer []byte) (int, error) {
	if err := r.entry.Validate(); err != nil {
		return 0, err
	}
	n, err := r.file.Read(buffer)
	if ownedTreeIOTestHook != nil {
		ownedTreeIOTestHook("source-read", filepath.Join(r.entry.scope.PathLabel(), r.entry.Name), n)
	}
	if changed := r.entry.Validate(); changed != nil {
		return n, changed
	}
	return n, err
}
func (r *OwnedReader) ReadAt(buffer []byte, offset int64) (int, error) {
	if err := r.entry.Validate(); err != nil {
		return 0, err
	}
	n, err := r.file.ReadAt(buffer, offset)
	if ownedTreeIOTestHook != nil {
		ownedTreeIOTestHook("source-read-at", filepath.Join(r.entry.scope.PathLabel(), r.entry.Name), n)
	}
	if changed := r.entry.Validate(); changed != nil {
		return n, changed
	}
	return n, err
}

// ChmodOwned applies legacy derived-marker permissions only to an opened file
// actually created by this tree; it never chmods existing data or a newcomer.
func (t *OwnedTree) ChmodOwned(relative string, mode os.FileMode) error {
	file, err := t.Open(relative)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.file.Chmod(mode.Perm()); err != nil {
		return err
	}
	return file.entry.Validate()
}

func (t *OwnedTree) Keep() { t.keep = true }
func (t *OwnedTree) Close() error {
	if t.root == nil {
		return nil
	}
	var result error
	if !t.keep {
		for i := len(t.created) - 1; i >= 0; i-- {
			result = errors.Join(result, t.created[i].scope.RemoveOwned(t.created[i]))
		}
	}
	for i := len(t.scopes) - 1; i >= 0; i-- {
		if t.keep {
			t.scopes[i].KeepDirectories()
		}
		result = errors.Join(result, t.scopes[i].Close())
	}
	t.root, t.scopes, t.dirs, t.files, t.created = nil, nil, nil, nil, nil
	return result
}
