// Package elasticsearch provides a high-level client for indexing and searching
// URL shortener data in Elasticsearch. It manages three index families:
//
//   - URLs index:   stores shortened URL documents for full-text search
//   - Clicks index: stores individual click events for analytics queries
//   - Logs index:   ships application logs via a buffered, batched writer
//
// All index names are prefixed with a configurable namespace (e.g. "tiny") so
// multiple environments can share a single Elasticsearch cluster without
// collisions. The package uses the official go-elasticsearch/v8 client and
// communicates via the Elasticsearch REST API.
package elasticsearch

import (
	"context"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// Config holds the connection and namespace settings for Elasticsearch.
// Enabled controls whether the application should attempt to connect at all,
// allowing graceful degradation when ES is not available.
type Config struct {
	Addresses   []string
	Username    string
	Password    string
	IndexPrefix string
	Enabled     bool
}

// Client wraps the official Elasticsearch client and adds index-prefix
// namespacing. All index operations (URL, click, log) route through this
// client so that the prefix is applied consistently.
type Client struct {
	es          *elasticsearch.Client
	indexPrefix string
}

// NewClient creates an Elasticsearch client, verifies connectivity by calling
// the cluster info endpoint, and returns the wrapped Client. The info call
// acts as a health check -- if the cluster is unreachable or returns an error
// status, the constructor fails fast so the caller knows ES is unavailable at
// startup rather than discovering it on the first query.
func NewClient(cfg Config) (*Client, error) {
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ping the cluster via the Info endpoint to verify connectivity.
	// The context is created for a future timeout guard but not yet wired
	// into the Info call (the v8 client's Info() does not accept a context).
	_ = ctx
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to elasticsearch: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch responded with error: %s", res.String())
	}

	return &Client{
		es:          es,
		indexPrefix: cfg.IndexPrefix,
	}, nil
}

// ES exposes the underlying go-elasticsearch client for callers that need
// direct access to low-level APIs not wrapped by this package.
func (c *Client) ES() *elasticsearch.Client {
	return c.es
}

// IndexPrefix returns the namespace prefix prepended to all index names
// (e.g. "tiny" produces indices like "tiny-urls", "tiny-clicks").
func (c *Client) IndexPrefix() string {
	return c.indexPrefix
}

// Close is a no-op provided to satisfy common closer interfaces. The
// underlying go-elasticsearch HTTP client is backed by net/http's default
// transport pool and does not require explicit shutdown.
func (c *Client) Close() error {
	return nil
}
