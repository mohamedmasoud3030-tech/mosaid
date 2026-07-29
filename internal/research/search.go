package research

import (
	"context"
	"errors"
	"strings"
)

type SearchService struct {
	Provider   SearchProvider
	ProviderID string
	URLPolicy  FetchPolicy
	Clock      Clock
}

func (s SearchService) Search(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	if s.Provider == nil || s.Clock == nil || strings.TrimSpace(s.ProviderID) == "" {
		return nil, errors.New("search adapter dependencies unavailable")
	}
	query.Text = strings.TrimSpace(query.Text)
	if query.Text == "" || len(query.Text) > 1000 || query.Limit < 1 || query.Limit > 20 {
		return nil, errors.New("invalid bounded search query")
	}
	providerResults, err := s.Provider.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(providerResults) > query.Limit {
		return nil, errors.New("search provider exceeded requested result limit")
	}
	policy := normalizedPolicy(s.URLPolicy)
	results := make([]SearchResult, 0, len(providerResults))
	for _, item := range providerResults {
		if len(item.Title) > 500 || len(item.Snippet) > 4000 {
			return nil, errors.New("search result exceeds text limits")
		}
		parsed, err := parseExternalURL(item.URL, policy)
		if err != nil {
			return nil, err
		}
		provenance := Provenance{URL: parsed.String(), RetrievedAt: s.Clock.Now().UTC(), SHA256: HashBytes([]byte(item.Snippet)), ContentType: "text/plain", Bytes: len(item.Snippet)}
		results = append(results, SearchResult{Title: item.Title, URL: parsed.String(), Snippet: NewExternalContent(item.Snippet, provenance), ProviderID: s.ProviderID})
	}
	return results, nil
}
