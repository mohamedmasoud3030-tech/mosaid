package images

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxArtifactBytes = 10 * 1024 * 1024

type Store struct{ root string }

func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: root required", ErrArtifact)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	if err = os.Chmod(absolute, 0o700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: root is not a directory", ErrArtifact)
	}
	return &Store{root: resolved}, nil
}

func (s *Store) Put(data []byte, mimeType string) (id, relativePath string, err error) {
	if len(data) == 0 || len(data) > maxArtifactBytes {
		return "", "", fmt.Errorf("%w: size", ErrArtifact)
	}
	extension, err := extensionForMIME(mimeType)
	if err != nil {
		return "", "", err
	}
	hash := sha256.Sum256(data)
	id = hex.EncodeToString(hash[:])
	name := id + extension
	destination := filepath.Join(s.root, name)
	if info, statErr := os.Lstat(destination); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("%w: existing path is unsafe", ErrArtifact)
		}
		existing, readErr := os.ReadFile(destination)
		if readErr != nil || !equalHash(existing, id) {
			return "", "", fmt.Errorf("%w: existing artifact mismatch", ErrArtifact)
		}
		return id, name, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", statErr
	}
	temporary, err := os.CreateTemp(s.root, ".image-*")
	if err != nil {
		return "", "", err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return "", "", err
	}
	if _, err = temporary.Write(data); err != nil {
		return "", "", err
	}
	if err = temporary.Sync(); err != nil {
		return "", "", err
	}
	if err = temporary.Close(); err != nil {
		return "", "", err
	}
	if err = os.Rename(temporaryName, destination); err != nil {
		return "", "", err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return "", "", err
	}
	err = directory.Sync()
	_ = directory.Close()
	if err != nil {
		return "", "", err
	}
	committed = true
	return id, name, nil
}

func (s *Store) Read(id string) (Reference, error) {
	if len(id) != 64 {
		return Reference{}, fmt.Errorf("%w: malformed id", ErrArtifact)
	}
	if _, err := hex.DecodeString(id); err != nil || strings.ToLower(id) != id {
		return Reference{}, fmt.Errorf("%w: malformed id", ErrArtifact)
	}
	for _, item := range []struct{ extension, mime string }{{".png", "image/png"}, {".jpg", "image/jpeg"}} {
		path := filepath.Join(s.root, id+item.extension)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxArtifactBytes {
			return Reference{}, fmt.Errorf("%w: unsafe artifact", ErrArtifact)
		}
		file, err := os.Open(path)
		if err != nil {
			return Reference{}, err
		}
		data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
		_ = file.Close()
		if err != nil || len(data) > maxArtifactBytes || !equalHash(data, id) || http.DetectContentType(data) != item.mime {
			return Reference{}, fmt.Errorf("%w: artifact integrity", ErrArtifact)
		}
		return Reference{ID: id, MIME: item.mime, SHA256: id, Data: data}, nil
	}
	return Reference{}, os.ErrNotExist
}

func extensionForMIME(mimeType string) (string, error) {
	switch mimeType {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	default:
		return "", fmt.Errorf("%w: MIME", ErrArtifact)
	}
}

func equalHash(data []byte, expected string) bool {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]) == expected
}
