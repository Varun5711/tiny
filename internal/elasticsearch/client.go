package elasticsearch

import (
	"context"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

type Config struct {
	Addresses   []string
	Username    string
	Password    string
	IndexPrefix string
	Enabled     bool
}

type Client struct {
	es          *elasticsearch.Client
	indexPrefix string
}

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

	_ = ctx
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to elasticsearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch responded with error: %s", res.String())
	}

	return &Client{
		es:          es,
		indexPrefix: cfg.IndexPrefix,
	}, nil
}

func (c *Client) ES() *elasticsearch.Client {
	return c.es
}

func (c *Client) IndexPrefix() string {
	return c.indexPrefix
}

func (c *Client) Close() error {
	return nil
}
