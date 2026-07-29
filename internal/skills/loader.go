package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxManifestBytes = 256 * 1024

type Loader struct {
	Root         string
	Capabilities Capabilities
}

func (l Loader) LoadFile(path string) (Manifest, error) {
	resolved, err := l.resolve(path)
	if err != nil {
		return Manifest{}, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: manifest too large", ErrInvalidManifest)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Manifest{}, fmt.Errorf("%w: trailing data", ErrInvalidManifest)
	}
	if err := manifest.Validate(l.Capabilities); err != nil {
		return Manifest{}, err
	}
	if err := manifest.VerifyIntegrity(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (l Loader) LoadFiles(paths []string, registry *Registry) error {
	if registry == nil {
		return errors.New("skill registry required")
	}
	for _, path := range paths {
		manifest, err := l.LoadFile(path)
		if err != nil {
			return fmt.Errorf("load %s: %w", filepath.Base(filepath.Dir(path)), err)
		}
		if err := registry.Add(manifest); err != nil {
			return err
		}
	}
	return nil
}

func (l Loader) resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New("manifest path required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: manifest must be a regular non-symlink file", ErrInvalidManifest)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if l.Root != "" {
		root, err := filepath.Abs(l.Root)
		if err != nil {
			return "", err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(root, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return "", fmt.Errorf("%w: manifest escapes skill root", ErrInvalidManifest)
		}
	}
	return resolved, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("malformed JSON array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
