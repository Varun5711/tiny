package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// URLDocument represents a shortened URL record stored in Elasticsearch.
// It mirrors the core URL fields from the primary datastore (PostgreSQL/Redis)
// so that Elasticsearch can serve full-text search without querying the
// transactional database. The ShortCode is used as the ES document ID,
// ensuring natural idempotency on re-index operations.
type URLDocument struct {
	ShortCode string     `json:"short_code"`
	LongURL   string     `json:"long_url"`
	UserID    string     `json:"user_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Clicks    int64      `json:"clicks"`
}

// URLSearchResult holds a page of URL search results together with the total
// number of matching documents, enabling pagination in the API layer.
type URLSearchResult struct {
	URLs  []URLDocument `json:"urls"`
	Total int64         `json:"total"`
}

// urlIndex returns the fully-qualified index name for URL documents
// (e.g. "tiny-urls"). Centralizing the name here ensures every URL
// operation targets the same index.
func (c *Client) urlIndex() string {
	return c.indexPrefix + "-urls"
}

// IndexURL indexes (creates or replaces) a URL document in Elasticsearch.
// The document ID is set to the ShortCode, which makes the operation
// idempotent -- re-indexing the same short code overwrites the previous
// version without creating duplicates.
//
// Refresh is set to "false" (the default) so writes are batched by ES's
// internal refresh interval (~1 s) rather than forcing an expensive
// per-document refresh. This trades sub-second search visibility for
// significantly higher write throughput.
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
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return fmt.Errorf("elasticsearch index error: %s", res.String())
	}

	return nil
}

// SearchURLs performs a full-text search across URL documents using the
// Elasticsearch multi_match query DSL.
//
// The query structure is:
//
//	{
//	  "multi_match": {
//	    "query":  "<user input>",
//	    "fields": ["long_url", "short_code"],
//	    "type":   "best_fields"
//	  }
//	}
//
// multi_match searches multiple fields simultaneously. The "best_fields" type
// picks the single best-matching field's score for each document (as opposed
// to "most_fields" which sums scores). This is ideal here because a match in
// either the original URL or the short code alone is sufficient relevance.
//
// Results are sorted by created_at descending so the most recent URLs appear
// first, and paginated via from/size.
func (c *Client) SearchURLs(ctx context.Context, query string, limit, offset int) (*URLSearchResult, error) {
	// Build the Elasticsearch Query DSL payload.
	// multi_match queries the user's text against both long_url and short_code
	// fields, returning the highest-scoring field match per document.
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
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search error: %s", res.String())
	}

	// Decode the nested Elasticsearch response envelope.
	// The structure is: { hits: { total: { value }, hits: [{ _source }] } }
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

// DeleteURL removes a URL document from the search index by its short code.
// A 404 response is silently ignored because the document may have already
// been deleted or never indexed -- this keeps the delete operation idempotent
// so callers do not need to check existence before deleting.
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
	defer func() { _ = res.Body.Close() }()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("elasticsearch delete error: %s", res.String())
	}

	return nil
}

// UpdateClicks performs a partial document update to set the click count for
// a URL. It uses the Elasticsearch Update API with a "doc" merge payload:
//
//	{ "doc": { "clicks": <new_value> } }
//
// The "doc" pattern tells Elasticsearch to merge only the specified fields
// into the existing document, leaving all other fields untouched. This is
// far more efficient than re-indexing the full document just to update a
// counter, because it avoids re-sending and re-analyzing fields like long_url.
//
// A 404 is ignored because the URL may have been deleted from the index
// between the time the click was recorded and this update ran.
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
	defer func() { _ = res.Body.Close() }()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("elasticsearch update error: %s", res.String())
	}

	return nil
}
