package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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

type ClickSearchFilters struct {
	ShortCode string
	Country   string
	Device    string
	StartDate *time.Time
	EndDate   *time.Time
}

type ClickSearchResult struct {
	Events []ClickEventDocument `json:"events"`
	Total  int64                `json:"total"`
}

func (c *Client) clickIndex() string {
	return c.indexPrefix + "-clicks"
}

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

func (c *Client) IndexClickEventsBulk(ctx context.Context, docs []ClickEventDocument) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": c.clickIndex(),
				"_id":    doc.EventID,
			},
		}
		metaLine, _ := json.Marshal(meta)
		buf.Write(metaLine)
		buf.WriteByte('\n')

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

func (c *Client) SearchClickEvents(ctx context.Context, filters ClickSearchFilters, limit, offset int) (*ClickSearchResult, error) {
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
