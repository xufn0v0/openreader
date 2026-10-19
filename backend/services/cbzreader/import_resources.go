package cbzreader

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"

	"openreader/backend/models"
	"openreader/backend/services/rootedfs"
)

// Initial import retains the caller's original tree throughout source reads,
// image writes and completion marker publication. Runtime caches stay separate.
func (s *Service) PrepareImportedResources(book models.Book, tree *rootedfs.OwnedTree) error {
	if !IsLocalCBZ(book) {
		return ErrNotCBZ
	}
	source, err := tree.Open(filepath.Base(book.OriginalFile))
	if err != nil {
		return err
	}
	defer source.Close()
	info := source.Info()
	if info.Size() < 0 || info.Size() > s.limits.MaxArchiveBytes {
		return ErrExtractionLimit
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(source, info.Size()+1))
	if err != nil {
		return err
	}
	if count != info.Size() {
		return ErrInvalidArchive
	}
	fingerprint := hex.EncodeToString(hash.Sum(nil))
	prefix := filepath.Join(extractionDirectoryName, fingerprint)
	if err := tree.Mkdir(prefix); err != nil {
		return err
	}
	reader, err := zip.NewReader(source, info.Size())
	if err != nil {
		return ErrInvalidArchive
	}
	cover, err := extractZipReaderOwned(reader, tree, prefix, s.limits)
	if err != nil {
		return err
	}
	marker, err := json.Marshal(extractionMarker{Fingerprint: fingerprint, Size: info.Size(), ModTimeNano: info.ModTime().UnixNano(), CoverPath: cover})
	if err != nil {
		return err
	}
	marker = append(marker, '\n')
	if _, err := tree.ReplaceOwned(filepath.Join(prefix, extractionMarkerName), bytes.NewReader(marker), int64(len(marker))); err != nil {
		return err
	}
	if err := tree.ChmodOwned(filepath.Join(prefix, extractionMarkerName), 0o600); err != nil {
		return err
	}
	return tree.Validate()
}
