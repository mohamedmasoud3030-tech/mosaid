package storage

import "testing"

func TestSession(t *testing.T) {
	s := NewSessionStore(t.TempDir())
	if e := s.Append(1, "user", "hi"); e != nil {
		t.Fatal(e)
	}
	m, e := s.Recent(1, 10)
	if e != nil || len(m) != 1 || m[0].Content != "hi" {
		t.Fatal(m, e)
	}
}
