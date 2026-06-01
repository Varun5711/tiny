package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ClickEventDocument represents a single URL click event stored in
// Elasticsearch. Each redirect through the shortener produces one of these
// documents, enriched with geo-IP and user-agent data by the redirect handler
// before indexing. The EventID is used as the ES document ID to guarantee
// exactly-once semantics on retries.
type ClickEventDocument struct {
	EventID     string    `json:"event_id"`
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	ClickedAt   time.Time `json:"clicked_at"`
	IPAddress   string    `json:"ip_address"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"`
	Region      string    `json:"region"`
	City        string    `json:"city"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	UserAgent   string    `json:"user_agent"`
	Browser     string    `json:"browser"`
	OS          string    `json:"os"`
	DeviceType  string    `json:"device_type"`
	Referer     string    `json:"referer"`
}

// ClickSearchFilters defines the optional filter criteria for searching click
// events. All fields are optional; when omitted, the corresponding filter
// clause is not added to the Elasticsearch query. This allows the API layer
// to build flexible filtered views (e.g. "all clicks from Germany on mobile
// devices in the last 7 days").
type ClickSearchFilters struct {
	ShortCode string
	Country   string
	Device    string
	StartDate *time.Time
	EndDate   *time.Time
}

// ClickSearchResult holds a page of click events together with the total
// match count for pagination.
type ClickSearchResult struct {
	Events []ClickEventDocument `json:"events"`
	Total  int64                `json:"total"`
}

// clickIndex returns the fully-qualified clicks index name (e.g. "tiny-clicks").
func (c *Client) clickIndex() string {
	return c.indexPrefix + "-clicks"
}

// IndexClickEvent indexes a single click event document. The EventID is used
// as the document ID, making the operation idempotent -- if an event is
// re-indexed due to a retry, it overwrites the previous version rather than
// creating a duplicate.
func (c *Client) IndexClickEvent(ctx context.Context, doc ClickEventDocument) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal click event: %w", err)
	}

	res, err := c.es.Index(
		c.clickIndex(),
		bytes.NewReader(body),
		c.es.Index.WithContext(ctx),
		c.es.Index.WithDocumentID(doc.EventID),
	)
	if err != nil {
		return fmt.Errorf("failed to index click event: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return fmt.Errorf("elasticsearch index error: %s", res.String())
	}

	return nil
}

// IndexClickEventsBulk indexes multiple click events in a single Elasticsearch
// Bulk API call. This is the preferred path for high-throughput ingestion
// because it amortizes the HTTP overhead across many documents.
//
// The Bulk API uses Newline-Delimited JSON (NDJSON) format, where each
// operation consists of two consecutive JSON lines:
//
//	Line 1 (action/metadata): {"index": {"_index": "<name>", "_id": "<id>"}}
//	Line 2 (document source): {"event_id": "...", "short_code": "...", ...}
//
// Each pair is separated by a newline (\n). The trailing newline after the
// last document is required by the Bulk API spec. Using "index" as the action
// means "create or replace", providing idempotency via the EventID.
func (c *Client) IndexClickEventsBulk(ctx context.Context, docs []ClickEventDocument) error {
	if len(docs) == 0 {
		return nil
	}

	// Build the NDJSON payload: alternating action-metadata and document lines.
	var buf bytes.Buffer
	for _, doc := range docs {
		// Action line: tells ES which index and document ID to target.
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": c.clickIndex(),
				"_id":    doc.EventID,
			},
		}
		metaLine, _ := json.Marshal(meta)
		buf.Write(metaLine)
		buf.WriteByte('\n')

		// Document line: the actual click event payload to store.
		dataLine, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal click event: %w", err)
		}
		buf.Write(dataLine)
		buf.WriteByte('\n')
	}

	res, err := c.es.Bulk(
		bytes.NewReader(buf.Bytes()),
		c.es.Bulk.WithContext(ctx),
		c.es.Bulk.WithIndex(c.clickIndex()),
	)
	if err != nil {
		return fmt.Errorf("failed to bulk index click events: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return fmt.Errorf("elasticsearch bulk index error: %s", res.String())
	}

	return nil
}

// SearchClickEvents searches click events using a compound bool/must query.
// Each non-empty filter adds a clause to the "must" array, so all specified
// filters must match (logical AND).
//
// The Elasticsearch Query DSL structure produced is:
//
//	{
//	  "query": {
//	    "bool": {
//	      "must": [
//	        { "term": { "short_code": "abc123" } },   // exact match filter
//	        { "term": { "country_code": "US" } },      // exact match filter
//	        { "term": { "device_type": "mobile" } },   // exact match filter
//	        { "range": { "clicked_at": { "gte": ..., "lte": ... } } }  // date range
//	      ]
//	    }
//	  }
//	}
//
// "term" queries perform exact (non-analyzed) matching, which is correct for
// keyword fields like short_code and country_code. The "range" query supports
// open-ended bounds -- either gte or lte can be omitted for one-sided ranges.
//
// When no filters are specified, a match_all query is used to return all
// click events (useful for admin dashboards).
func (c *Client) SearchClickEvents(ctx context.Context, filters ClickSearchFilters, limit, offset int) (*ClickSearchResult, error) {
	// Build the bool/must clause list dynamically based on which filters
	// the caller provided. Each non-empty filter becomes a "must" clause.
	var musts []map[string]interface{}

	if filters.ShortCode != "" {
		musts = append(musts, map[string]interface{}{
			"term": map[string]interface{}{"short_code": filters.ShortCode},
		})
	}

	if filters.Country != "" {
		musts = append(musts, map[string]interface{}{
			"term": map[string]interface{}{"country_code": filters.Country},
		})
	}

	if filters.Device != "" {
		musts = append(musts, map[string]interface{}{
			"term": map[string]interface{}{"device_type": filters.Device},
		})
	}

	// Range filter for clicked_at: supports open-ended bounds (gte-only,
	// lte-only, or both) so callers can query "since date X" or "before
	// date Y" without needing to specify both ends.
	if filters.StartDate != nil || filters.EndDate != nil {
		rangeFilter := map[string]interface{}{}
		if filters.StartDate != nil {
			rangeFilter["gte"] = filters.StartDate.Format(time.RFC3339)
		}
		if filters.EndDate != nil {
			rangeFilter["lte"] = filters.EndDate.Format(time.RFC3339)
		}
		musts = append(musts, map[string]interface{}{
			"range": map[string]interface{}{"clicked_at": rangeFilter},
		})
	}

	query := map[string]interface{}{
		"from": offset,
		"size": limit,
		"sort": []map[string]interface{}{
			{"clicked_at": map[string]string{"order": "desc"}},
		},
	}

	// Use a bool/must compound query when filters exist; otherwise fall
	// back to match_all to return everything (paginated by from/size).
	if len(musts) > 0 {
		query["query"] = map[string]interface{}{
			"bool": map[string]interface{}{"must": musts},
		}
	} else {
		query["query"] = map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search body: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.clickIndex()),
		c.es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search click events: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch search error: %s", res.String())
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source ClickEventDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	events := make([]ClickEventDocument, len(result.Hits.Hits))
	for i, hit := range result.Hits.Hits {
		events[i] = hit.Source
	}

	return &ClickSearchResult{
		Events: events,
		Total:  result.Hits.Total.Value,
	}, nil
}
