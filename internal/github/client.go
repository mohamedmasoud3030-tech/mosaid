package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Auth interface {
	Token(context.Context) (string, error)
}
type StaticToken string

func (s StaticToken) Token(context.Context) (string, error) {
	if s == "" {
		return "", errors.New("missing token")
	}
	return string(s), nil
}

type Client struct {
	Base string
	Auth Auth
	HTTP *http.Client
	Max  int64
}

func New(a Auth) *Client {
	return &Client{Base: "https://api.github.com", Auth: a, HTTP: &http.Client{Timeout: 30 * time.Second}, Max: 1 << 20}
}
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	if !strings.HasPrefix(path, "/") {
		return errors.New("bad path")
	}
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, body)
	tok, e := c.Auth.Token(ctx)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, c.Max+1))
	if e != nil {
		return e
	}
	if int64(len(b)) > c.Max {
		return errors.New("GitHub response too large")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

type Repository struct {
	FullName, DefaultBranch string
	Private                 bool
	Description             string
}

func (c *Client) Repository(ctx context.Context, owner, repo string) (Repository, error) {
	var r struct {
		FullName    string `json:"full_name"`
		Default     string `json:"default_branch"`
		Private     bool   `json:"private"`
		Description string `json:"description"`
	}
	e := c.do(ctx, "GET", "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(repo), nil, &r)
	return Repository{r.FullName, r.Default, r.Private, r.Description}, e
}

type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
}

func (c *Client) Issues(ctx context.Context, owner, repo string) ([]Issue, error) {
	var x []Issue
	e := c.do(ctx, "GET", "/repos/"+owner+"/"+repo+"/issues?per_page=50", nil, &x)
	return x, e
}

type Pull struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	Draft  bool   `json:"draft"`
}

func (c *Client) CreateDraftPR(ctx context.Context, owner, repo, title, head, base, body string) (Pull, error) {
	var p Pull
	e := c.do(ctx, "POST", "/repos/"+owner+"/"+repo+"/pulls", map[string]any{"title": title, "head": head, "base": base, "body": body, "draft": true}, &p)
	return p, e
}
func (c *Client) UpdatePR(ctx context.Context, owner, repo string, n int, body string) error {
	return c.do(ctx, "PATCH", "/repos/"+owner+"/"+repo+"/pulls/"+strconv.Itoa(n), map[string]string{"body": body}, nil)
}
func (c *Client) WorkflowRuns(ctx context.Context, owner, repo, branch string) (json.RawMessage, error) {
	var r json.RawMessage
	e := c.do(ctx, "GET", "/repos/"+owner+"/"+repo+"/actions/runs?branch="+url.QueryEscape(branch)+"&per_page=20", nil, &r)
	return r, e
}
