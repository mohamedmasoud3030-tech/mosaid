package instagram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type TokenSource interface {
	BearerToken(context.Context) (string, error)
}

type MetaConfig struct {
	APIVersion       string
	Tokens           TokenSource
	Client           *http.Client
	MaxResponseBytes int64
}

type GraphClient struct{ config MetaConfig }

var apiVersionPattern = regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,2}$`)

func NewGraphClient(config MetaConfig) (*GraphClient, error) {
	if !apiVersionPattern.MatchString(config.APIVersion) || config.Tokens == nil {
		return nil, errors.New("meta Graph API version and token source required")
	}
	if config.Client == nil {
		config.Client = &http.Client{}
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = 1024 * 1024
	}
	if config.MaxResponseBytes < 1024 || config.MaxResponseBytes > 4*1024*1024 {
		return nil, errors.New("meta response limit invalid")
	}
	return &GraphClient{config: config}, nil
}

func (c *GraphClient) CreateContainer(ctx context.Context, accountID, imageURL, caption, idempotencyKey string) (string, error) {
	if !accountPattern.MatchString(accountID) || idempotencyKey == "" {
		return "", ErrInvalidDraft
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return "", ErrInvalidDraft
	}
	form := url.Values{"image_url": {imageURL}, "caption": {caption}}
	var response struct {
		ID string `json:"id"`
	}
	if err = c.request(ctx, http.MethodPost, "/"+accountID+"/media", form, idempotencyKey, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", ErrContainer
	}
	return response.ID, nil
}

func (c *GraphClient) ContainerStatus(ctx context.Context, containerID string) (ContainerStatus, error) {
	if containerID == "" || len(containerID) > 256 {
		return "", ErrContainer
	}
	var response struct {
		Status ContainerStatus `json:"status_code"`
	}
	if err := c.request(ctx, http.MethodGet, "/"+url.PathEscape(containerID), url.Values{"fields": {"status_code"}}, "", &response); err != nil {
		return "", err
	}
	switch response.Status {
	case ContainerInProgress, ContainerFinished, ContainerError, ContainerExpired:
		return response.Status, nil
	default:
		return "", ErrContainer
	}
}

func (c *GraphClient) PublishContainer(ctx context.Context, accountID, containerID, idempotencyKey string) (string, error) {
	if !accountPattern.MatchString(accountID) || containerID == "" || idempotencyKey == "" {
		return "", ErrInvalidDraft
	}
	form := url.Values{"creation_id": {containerID}}
	var response struct {
		ID string `json:"id"`
	}
	if err := c.request(ctx, http.MethodPost, "/"+accountID+"/media_publish", form, idempotencyKey, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", ErrContainer
	}
	return response.ID, nil
}

func (c *GraphClient) request(ctx context.Context, method, path string, values url.Values, idempotencyKey string, target any) error {
	token, err := c.config.Tokens.BearerToken(ctx)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n") {
		return errors.New("meta credential unavailable")
	}
	endpoint := "https://graph.facebook.com/" + c.config.APIVersion + path
	var body io.Reader
	if method == http.MethodGet {
		endpoint += "?" + values.Encode()
	} else {
		body = strings.NewReader(values.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := *c.config.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("meta redirects disabled") }
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("meta Graph API HTTP status %d", response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.config.MaxResponseBytes+1))
	if err != nil || int64(len(encoded)) > c.config.MaxResponseBytes {
		return errors.New("meta Graph API response too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return errors.New("invalid Meta Graph API response")
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid Meta Graph API response")
	}
	return nil
}
