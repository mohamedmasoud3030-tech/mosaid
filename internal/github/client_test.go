package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRepositoryAndDraftPR(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Fatal("auth")
		}
		switch {
		case r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"full_name": "o/r", "default_branch": "main", "private": true})
		case r.Method == "POST":
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["draft"] != true {
				t.Fatal("not draft")
			}
			json.NewEncoder(w).Encode(map[string]any{"number": 1, "html_url": "https://example.invalid/pr/1", "draft": true})
		}
	}))
	defer s.Close()
	c := New(StaticToken("test"))
	c.Base = s.URL
	r, e := c.Repository(context.Background(), "o", "r")
	if e != nil || r.FullName != "o/r" {
		t.Fatal(r, e)
	}
	p, e := c.CreateDraftPR(context.Background(), "o", "r", "t", "h", "main", "b")
	if e != nil || !p.Draft {
		t.Fatal(p, e)
	}
}
