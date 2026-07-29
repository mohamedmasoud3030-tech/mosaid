package secrets

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const maxSecretBytes = 64 * 1024

type Source interface {
	Read(string) (*Value, error)
}

type Value struct{ bytes []byte }

func (v *Value) String() string {
	if v == nil {
		return ""
	}
	return string(v.bytes)
}

func (v *Value) Destroy() {
	if v == nil {
		return
	}
	for index := range v.bytes {
		v.bytes[index] = 0
	}
	v.bytes = nil
}

type FileSource struct{}

func (FileSource) Read(path string) (*Value, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("secret must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("secret file permissions too broad: %o", info.Mode().Perm())
	}
	if info.Size() < 1 || info.Size() > maxSecretBytes {
		return nil, errors.New("secret file size invalid")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	value := strings.TrimSpace(string(bytes))
	for index := range bytes {
		bytes[index] = 0
	}
	if value == "" || strings.ContainsRune(value, 0) || strings.ContainsAny(value, "\r\n") {
		return nil, errors.New("secret must be one non-empty line")
	}
	return &Value{bytes: []byte(value)}, nil
}
