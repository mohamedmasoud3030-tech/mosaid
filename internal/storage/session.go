package storage

import (
	"bufio"
	"encoding/json"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SessionStore struct {
	mu   sync.Mutex
	path string
}
type record struct {
	ChatID  int64     `json:"chat_id"`
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

func NewSessionStore(dir string) *SessionStore {
	return &SessionStore{path: filepath.Join(dir, "sessions.jsonl")}
}
func (s *SessionStore) Append(chat int64, role, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(record{chat, role, content, time.Now().UTC()})
}
func (s *SessionStore) Recent(chat int64, limit int) ([]model.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []model.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.ChatID == chat {
			all = append(all, model.Message{Role: r.Role, Content: r.Content})
		}
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, sc.Err()
}
