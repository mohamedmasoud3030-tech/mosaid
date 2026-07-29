package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Workspace struct {
	Root, TrashPath    string
	MaxFile, MaxOutput int64
}

func NewWorkspace(root string) *Workspace {
	return &Workspace{Root: filepath.Clean(root), TrashPath: filepath.Join(root, ".mosaid-trash"), MaxFile: 1 << 20, MaxOutput: 1 << 20}
}
func (w *Workspace) path(rel string, write bool) (string, error) {
	if filepath.IsAbs(rel) || rel == "" {
		return "", errors.New("relative path required")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("path traversal")
	}
	for _, part := range strings.Split(clean, string(os.PathSeparator)) {
		l := strings.ToLower(part)
		if l == ".env" || strings.HasPrefix(l, ".env.") || l == ".security.yml" || l == "credentials" || l == "secrets" {
			return "", errors.New("secret path denied")
		}
	}
	p := filepath.Join(w.Root, clean)
	parent := p
	if write {
		parent = filepath.Dir(p)
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if write && os.IsNotExist(err) {
			resolved = parent
		} else {
			return "", err
		}
	}
	root, err := filepath.EvalSymlinks(w.Root)
	if err != nil {
		root = w.Root
	}
	relcheck, err := filepath.Rel(root, resolved)
	if err != nil || relcheck == ".." || strings.HasPrefix(relcheck, ".."+string(os.PathSeparator)) {
		return "", errors.New("workspace escape")
	}
	if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symlink target denied")
	}
	return p, nil
}
func (w *Workspace) Read(_ context.Context, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(args, &a) != nil {
		return nil, errors.New("bad args")
	}
	p, e := w.path(a.Path, false)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, w.MaxFile+1))
	if int64(len(b)) > w.MaxFile {
		return nil, errors.New("file too large")
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return map[string]any{"binary": true, "size": len(b)}, nil
	}
	return string(b), e
}
func (w *Workspace) List(_ context.Context, args json.RawMessage) (any, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &a)
	if a.Path == "" {
		a.Path = "."
	}
	p, e := w.path(a.Path, false)
	if e != nil {
		return nil, e
	}
	es, e := os.ReadDir(p)
	if e != nil {
		return nil, e
	}
	out := make([]map[string]any, 0, len(es))
	for _, x := range es {
		info, _ := x.Info()
		out = append(out, map[string]any{"name": x.Name(), "dir": x.IsDir(), "size": info.Size()})
	}
	return out, nil
}
func (w *Workspace) Write(_ context.Context, args json.RawMessage) (any, error) {
	var a struct{ Path, Content string }
	if json.Unmarshal(args, &a) != nil {
		return nil, errors.New("bad args")
	}
	if int64(len(a.Content)) > w.MaxFile {
		return nil, errors.New("content too large")
	}
	p, e := w.path(a.Path, true)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".mosaid-write-")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	tmp.Chmod(0600)
	if _, err = tmp.WriteString(a.Content); err == nil {
		err = tmp.Sync()
	}
	if ce := tmp.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return nil, err
	}
	if old, er := os.ReadFile(p); er == nil {
		_ = os.WriteFile(p+".mosaid-backup", old, 0600)
	}
	if err = os.Rename(name, p); err != nil {
		return nil, err
	}
	return map[string]any{"path": a.Path, "bytes": len(a.Content)}, nil
}
func (w *Workspace) Patch(ctx context.Context, args json.RawMessage) (any, error) {
	var a struct{ Path, Old, New string }
	if json.Unmarshal(args, &a) != nil || a.Old == "" {
		return nil, errors.New("bad args")
	}
	v, e := w.Read(ctx, json.RawMessage(fmt.Sprintf(`{"path":%q}`, a.Path)))
	if e != nil {
		return nil, e
	}
	s, ok := v.(string)
	if !ok {
		return nil, errors.New("binary patch denied")
	}
	if strings.Count(s, a.Old) != 1 {
		return nil, errors.New("patch requires exactly one match")
	}
	b, _ := json.Marshal(map[string]string{"Path": a.Path, "Content": strings.Replace(s, a.Old, a.New, 1)})
	return w.Write(ctx, b)
}
func (w *Workspace) Mkdir(_ context.Context, args json.RawMessage) (any, error) {
	var a struct{ Path string }
	_ = json.Unmarshal(args, &a)
	p, e := w.path(a.Path, true)
	if e != nil {
		return nil, e
	}
	return a.Path, os.MkdirAll(p, 0700)
}
func (w *Workspace) Trash(_ context.Context, args json.RawMessage) (any, error) {
	var a struct{ Path string }
	_ = json.Unmarshal(args, &a)
	p, e := w.path(a.Path, true)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(w.TrashPath, 0700); e != nil {
		return nil, e
	}
	dst := filepath.Join(w.TrashPath, fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(p)))
	if e = os.Rename(p, dst); e != nil {
		return nil, e
	}
	return filepath.Base(dst), nil
}
func (w *Workspace) Search(_ context.Context, args json.RawMessage) (any, error) {
	var a struct{ Path, Query string }
	_ = json.Unmarshal(args, &a)
	if a.Path == "" {
		a.Path = "."
	}
	p, e := w.path(a.Path, false)
	if e != nil {
		return nil, e
	}
	var out []string
	e = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		f, er := os.Open(path)
		if er != nil {
			return nil
		}
		defer f.Close()
		s := bufio.NewScanner(io.LimitReader(f, w.MaxFile))
		for n := 1; s.Scan(); n++ {
			if strings.Contains(s.Text(), a.Query) {
				rel, _ := filepath.Rel(w.Root, path)
				out = append(out, fmt.Sprintf("%s:%d:%s", rel, n, s.Text()))
				if len(out) >= 100 {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return out, e
}
