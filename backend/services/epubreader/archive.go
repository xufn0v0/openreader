package epubreader

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"openreader/backend/services/rootedfs"
)

type extractionLimits struct {
	MaxArchiveBytes int64
	MaxEntries      int
	MaxEntryBytes   int64
	MaxTotalBytes   int64
}

// Nil outside lifecycle fixtures; called only after an actual entry write and
// source close. It never changes ordinary archive parsing or I/O.
var afterArchiveEntryTestHook func(destination, canonical string)

func defaultExtractionLimits() extractionLimits {
	return extractionLimits{
		MaxArchiveBytes: 1 << 30,
		MaxEntries:      20_000,
		MaxEntryBytes:   128 << 20,
		MaxTotalBytes:   2 << 30,
	}
}

func extractArchive(data []byte, destination string, limits extractionLimits) error {
	if int64(len(data)) > limits.MaxArchiveBytes {
		return fmt.Errorf("%w: compressed archive is too large", ErrExtractionLimit)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: invalid zip archive", ErrInvalidArchive)
	}
	return extractZipReader(reader, destination, limits)
}

func extractArchiveFile(sourcePath, destination string, limits extractionLimits) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > limits.MaxArchiveBytes {
		return fmt.Errorf("%w: compressed archive is too large", ErrExtractionLimit)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("%w: invalid zip archive", ErrInvalidArchive)
	}
	return extractZipReader(reader, destination, limits)
}

type archiveEntry struct {
	file      *zip.File
	canonical string
	isDir     bool
}

func extractZipReader(reader *zip.Reader, destination string, limits extractionLimits) error {
	tree, err := rootedfs.OpenOwnedTree(context.Background(), destination, ".")
	if err != nil {
		return err
	}
	defer tree.Close()
	if err := extractZipReaderOwned(reader, tree, ".", limits); err != nil {
		return err
	}
	tree.Keep()
	return nil
}

func extractZipReaderOwned(reader *zip.Reader, tree *rootedfs.OwnedTree, prefix string, limits extractionLimits) error {
	if err := tree.Validate(); err != nil {
		return err
	}
	if limits.MaxEntryBytes < 0 || limits.MaxTotalBytes < 0 {
		return ErrExtractionLimit
	}
	if len(reader.File) > limits.MaxEntries {
		return fmt.Errorf("%w: too many archive entries", ErrExtractionLimit)
	}

	entries := make([]archiveEntry, 0, len(reader.File))
	seen := make(map[string]bool, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		if err := tree.Context().Err(); err != nil {
			return err
		}
		canonical, err := normalizeArchivePath(file.Name)
		if err != nil {
			return err
		}
		if canonical == "" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic links are not allowed", ErrUnsafePath)
		}
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate archive path", ErrUnsafePath)
		}
		for parent := path.Dir(canonical); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if parentIsDir, exists := seen[strings.ToLower(parent)]; exists && !parentIsDir {
				return fmt.Errorf("%w: archive path conflicts with a file", ErrUnsafePath)
			}
		}
		if !isDir {
			prefix := key + "/"
			for existing := range seen {
				if strings.HasPrefix(existing, prefix) {
					return fmt.Errorf("%w: archive file conflicts with a directory", ErrUnsafePath)
				}
			}
		}
		seen[key] = isDir

		if !isDir {
			if file.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
				return fmt.Errorf("%w: archive entry is too large", ErrExtractionLimit)
			}
			if ^uint64(0)-total < file.UncompressedSize64 {
				return fmt.Errorf("%w: archive size overflow", ErrExtractionLimit)
			}
			total += file.UncompressedSize64
			if total > uint64(limits.MaxTotalBytes) {
				return fmt.Errorf("%w: archive expands beyond the total limit", ErrExtractionLimit)
			}
		}
		entries = append(entries, archiveEntry{file: file, canonical: canonical, isDir: isDir})
	}

	var actualTotal int64
	for _, entry := range entries {
		if err := tree.Context().Err(); err != nil {
			return err
		}
		target := filepath.Join(prefix, filepath.FromSlash(entry.canonical))
		if entry.isDir {
			if err := tree.Mkdir(target); err != nil {
				return err
			}
			continue
		}
		opened, err := entry.file.Open()
		if err != nil {
			return err
		}
		written, writeErr := tree.Write(target, opened, limits.MaxEntryBytes)
		closeErr := opened.Close()
		if writeErr != nil {
			if writeErr == rootedfs.ErrWriteLimit {
				return ErrExtractionLimit
			}
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > limits.MaxTotalBytes-actualTotal {
			return ErrExtractionLimit
		}
		actualTotal += written
		if afterArchiveEntryTestHook != nil {
			afterArchiveEntryTestHook(filepath.Join(tree.PathLabel(), prefix), entry.canonical)
		}
	}
	return tree.Validate()
}

func normalizeArchivePath(name string) (string, error) {
	if strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: malformed archive path", ErrUnsafePath)
	}
	if strings.HasPrefix(name, "/") || hasWindowsDrivePrefix(name) {
		return "", fmt.Errorf("%w: absolute archive path", ErrUnsafePath)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: parent traversal", ErrUnsafePath)
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: archive path escaped root", ErrUnsafePath)
	}
	return cleaned, nil
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':'
}

func extractionTarget(root, archivePath string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(archivePath))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: extraction path escaped root", ErrUnsafePath)
	}
	return target, nil
}

func writeBoundedFile(target string, source io.Reader, maxBytes int64) error {
	tree, err := rootedfs.OpenOwnedTree(context.Background(), filepath.Dir(target), ".")
	if err != nil {
		return err
	}
	defer tree.Close()
	if _, err := tree.Write(filepath.Base(target), source, maxBytes); err != nil {
		if err == rootedfs.ErrWriteLimit {
			return ErrExtractionLimit
		}
		return err
	}
	tree.Keep()
	return nil
}
