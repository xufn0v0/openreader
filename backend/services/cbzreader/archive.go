package cbzreader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/services/rootedfs"
)

type extractionLimits struct {
	MaxArchiveBytes int64
	MaxEntries      int
	MaxEntryBytes   int64
	MaxTotalBytes   int64
}

// Nil outside deterministic actual-entry ownership fixtures.
var afterArchiveEntryTestHook func(destination, canonical string)

func extractionLimitsFromConfig(_ config.Config) extractionLimits {
	// New imports already pass the caller-configured strict parser limits before
	// an archive is allocated. Runtime extraction deliberately retains the
	// bounded legacy limits so an existing valid mounted CBZ does not become
	// unreadable merely because the default upload limit is lower.
	return extractionLimits{
		MaxArchiveBytes: maxCBZArchiveBytes,
		MaxEntries:      maxCBZEntries,
		MaxEntryBytes:   maxCBZEntryBytes,
		MaxTotalBytes:   maxCBZTotalBytes,
	}
}

type archiveEntry struct {
	file      *zip.File
	canonical string
	isDir     bool
	isImage   bool
}

func extractArchiveFile(sourcePath, destination string, limits extractionLimits) (string, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > limits.MaxArchiveBytes {
		return "", fmt.Errorf("%w: compressed archive is too large", ErrExtractionLimit)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return "", fmt.Errorf("%w: invalid zip archive", ErrInvalidArchive)
	}
	return extractZipReader(reader, destination, limits)
}

func extractZipReader(reader *zip.Reader, destination string, limits extractionLimits) (string, error) {
	tree, err := rootedfs.OpenOwnedTree(context.Background(), destination, ".")
	if err != nil {
		return "", err
	}
	defer tree.Close()
	cover, err := extractZipReaderOwned(reader, tree, ".", limits)
	if err != nil {
		return "", err
	}
	tree.Keep()
	return cover, nil
}

func extractZipReaderOwned(reader *zip.Reader, tree *rootedfs.OwnedTree, prefix string, limits extractionLimits) (coverPath string, returnErr error) {
	if err := tree.Validate(); err != nil {
		return "", err
	}
	if limits.MaxEntryBytes < 0 || limits.MaxTotalBytes < 0 {
		return "", ErrExtractionLimit
	}
	if len(reader.File) > limits.MaxEntries {
		return "", fmt.Errorf("%w: too many archive entries", ErrExtractionLimit)
	}

	entries := make([]archiveEntry, 0, len(reader.File))
	seen := make(map[string]bool, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		if err := tree.Context().Err(); err != nil {
			return "", err
		}
		canonical, err := engine.NormalizeCBZResourcePath(file.Name)
		if err != nil {
			return "", fmt.Errorf("%w: malformed archive path", ErrUnsafePath)
		}
		if canonical == "" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symbolic links are not allowed", ErrUnsafePath)
		}
		isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		key := strings.ToLower(canonical)
		if _, exists := seen[key]; exists {
			return "", fmt.Errorf("%w: duplicate archive path", ErrUnsafePath)
		}
		for parent := path.Dir(canonical); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if parentIsDir, exists := seen[strings.ToLower(parent)]; exists && !parentIsDir {
				return "", fmt.Errorf("%w: archive path conflicts with a file", ErrUnsafePath)
			}
		}
		if !isDir {
			prefix := key + "/"
			for existing := range seen {
				if strings.HasPrefix(existing, prefix) {
					return "", fmt.Errorf("%w: archive file conflicts with a directory", ErrUnsafePath)
				}
			}
		}
		seen[key] = isDir

		if !isDir {
			if file.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
				return "", fmt.Errorf("%w: archive entry is too large", ErrExtractionLimit)
			}
			if ^uint64(0)-total < file.UncompressedSize64 {
				return "", fmt.Errorf("%w: archive size overflow", ErrExtractionLimit)
			}
			total += file.UncompressedSize64
			if total > uint64(limits.MaxTotalBytes) {
				return "", fmt.Errorf("%w: archive expands beyond the total limit", ErrExtractionLimit)
			}
		}
		_, isImage := engine.CBZImageContentType(canonical)
		if !isDir && isImage && coverPath == "" {
			coverPath = canonical
		}
		entries = append(entries, archiveEntry{file: file, canonical: canonical, isDir: isDir, isImage: isImage})
	}
	if coverPath == "" {
		return "", ErrNotFound
	}

	var actualTotal int64
	for _, entry := range entries {
		if err := tree.Context().Err(); err != nil {
			return "", err
		}
		if entry.isDir || !entry.isImage {
			continue
		}
		target := filepath.Join(prefix, filepath.FromSlash(entry.canonical))
		opened, err := entry.file.Open()
		if err != nil {
			return "", err
		}
		written, writeErr := tree.Write(target, opened, limits.MaxEntryBytes)
		closeErr := opened.Close()
		if writeErr != nil {
			if writeErr == rootedfs.ErrWriteLimit {
				return "", ErrExtractionLimit
			}
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if written > limits.MaxTotalBytes-actualTotal {
			return "", fmt.Errorf("%w: extracted files exceed the total limit", ErrExtractionLimit)
		}
		actualTotal += written
		if afterArchiveEntryTestHook != nil {
			afterArchiveEntryTestHook(filepath.Join(tree.PathLabel(), prefix), entry.canonical)
		}
	}
	return coverPath, tree.Validate()
}

func extractionTarget(root, archivePath string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(archivePath))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: extraction path escaped root", ErrUnsafePath)
	}
	return target, nil
}

func writeBoundedFile(target string, source io.Reader, maxBytes int64) (int64, error) {
	tree, err := rootedfs.OpenOwnedTree(context.Background(), filepath.Dir(target), ".")
	if err != nil {
		return 0, err
	}
	defer tree.Close()
	written, err := tree.Write(filepath.Base(target), source, maxBytes)
	if err != nil {
		if err == rootedfs.ErrWriteLimit {
			return written, ErrExtractionLimit
		}
		return written, err
	}
	tree.Keep()
	return written, nil
}
