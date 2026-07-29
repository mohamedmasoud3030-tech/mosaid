package health

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type State struct {
	StartedAt    time.Time `json:"started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Status       string    `json:"status"`
	Version      string    `json:"version"`
	LastUpdateID int64     `json:"last_update_id"`
	Messages     uint64    `json:"messages"`
}
type Writer struct {
	mu   sync.Mutex
	path string
	s    State
}

func New(dir, version string) *Writer {
	return &Writer{path: filepath.Join(dir, "health.json"), s: State{StartedAt: time.Now().UTC(), Status: "starting", Version: version}}
}
func (w *Writer) Update(fn func(*State)) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(&w.s)
	w.s.UpdatedAt = time.Now().UTC()
	b, _ := json.MarshalIndent(w.s, "", "  ")
	if err := os.MkdirAll(filepath.Dir(w.path), 0700); err != nil {
		return err
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, w.path)
}
func (w *Writer) Snapshot() State { w.mu.Lock(); defer w.mu.Unlock(); return w.s }
