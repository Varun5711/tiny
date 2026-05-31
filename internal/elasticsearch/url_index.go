package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type URLDocument struct {
	ShortCode string     `json:"short_code"`
	LongURL   string     `json:"long_url"`
	UserID    string     `json:"user_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Clicks    int64      `json:"clicks"`
}

type URLSearchResult struct {
	URLs  []URLDocument `json:"urls"`
	Total int64         `json:"total"`
}

func (c *Client) urlIndex() string {
	return c.indexPrefix + "-urls"
}

func (c *Client) IndexURL(ctx context.Context, doc URLDocument) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal URL document: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      c.urlIndex(),
		DocumentID: doc.ShortCode,
		Body:       bytes.NewReader(body),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to index URL: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch index error: %s", res.String())
	}

	return nil
}

func (c *Client) SearchURLs(ctx context.Context, query string, limit, offset int) (*URLSearchResult, error) {
	searchBody := map[string]interface{}{
		"from": offset,
		"size": limit,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"long_url", "short_code"},
				"type":   "best_fields",
			},
		},
		"sort": []map[string]interface{}{
			{"created_at": map[string]string{"order": "desc"}},
		},
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search body: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.urlIndex()),
		c.es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search URLs: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search error: %s", res.String())
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source URLDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	urls := make([]URLDocument, len(result.Hits.Hits))
	for i, hit := range result.Hits.Hits {
		urls[i] = hit.Source
	}

	return &URLSearchResult{
		URLs:  urls,
		Total: result.Hits.Total.Value,
	}, nil
}

func (c *Client) DeleteURL(ctx context.Context, shortCode string) error {
	req := esapi.DeleteRequest{
		Index:      c.urlIndex(),
		DocumentID: shortCode,
		Refresh:    "false",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to delete URL from index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("elasticsearch delete error: %s", res.String())
	}

	return nil
}

func (c *Client) UpdateClicks(ctx context.Context, shortCode string, clicks int64) error {
	body := map[string]interface{}{
		"doc": map[string]interface{}{
			"clicks": clicks,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	req := esapi.UpdateRequest{
		Index:      c.urlIndex(),
		DocumentID: shortCode,
		Body:       bytes.NewReader(data),
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return fmt.Errorf("failed to update clicks: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("elasticsearch update error: %s", res.String())
	}

	return nil
}
