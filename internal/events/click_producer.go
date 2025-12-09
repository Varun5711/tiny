package events

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type ClickProducer struct {
	client     *redis.Client
	streamName string
}

// NewClickProducer creates a new click event producer
func NewClickProducer(client *redis.Client, streamName string) *ClickProducer {
	return &ClickProducer{
		client:     client,
		streamName: streamName,
	}
}

func (p *ClickProducer) Publish(ctx context.Context, event *ClickEvent) error {
	fields := map[string]interface{}{
		"short_code": event.ShortCode,
		"timestamp":  event.Timestamp,
	}

	if event.IP != "" {
		fields["ip"] = event.IP
	}
	if event.UserAgent != "" {
		fields["user_agent"] = event.UserAgent
	}

	result := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamName,
		Values: fields,
	})

	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to publish click event: %w", err)
	}

	return nil
}

func (p *ClickProducer) PublishBatch(ctx context.Context, events []*ClickEvent) error {
	pipe := p.client.Pipeline()

	for _, event := range events {
		fields := map[string]interface{}{
			"short_code": event.ShortCode,
			"timestamp":  event.Timestamp,
		}

		if event.IP != "" {
			fields["ip"] = event.IP
		}
		if event.UserAgent != "" {
			fields["user_agent"] = event.UserAgent
		}

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: p.streamName,
			Values: fields,
		})
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish batch: %w", err)
	}

	return nil
}

func (p *ClickProducer) StreamInfo(ctx context.Context) (map[string]interface{}, error) {
	info := p.client.XInfoStream(ctx, p.streamName)
	if err := info.Err(); err != nil {
		return nil, err
	}

	result := info.Val()
	return map[string]interface{}{
		"length":      result.Length,
		"first_entry": result.FirstEntry,
		"last_entry":  result.LastEntry,
		"groups":      result.Groups,
	}, nil
}

func (p *ClickProducer) StreamLength(ctx context.Context) (int64, error) {
	result := p.client.XLen(ctx, p.streamName)
	return result.Val(), result.Err()
}
