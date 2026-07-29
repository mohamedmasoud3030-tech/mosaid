package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Fatal("auth")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer s.Close()
	c := NewOpenAI(s.URL, "k", "m", time.Second, 1024)
	c.base = s.URL
	if v, e := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}); e != nil || v != "ok" {
		t.Fatal(v, e)
	}
}
