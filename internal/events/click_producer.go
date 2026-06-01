package events

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ClickProducer publishes ClickEvents to a Redis Stream using XADD.
//
// It is used on the hot redirect path: after resolving a short code the HTTP
// handler calls Publish (or PublishBatch) to record the click asynchronously.
// Because XADD is an O(1) append, the additional latency on the redirect
// response is negligible -- typically under 1 ms on a local Redis.
//
// Empty optional fields are omitted from the stream entry to save memory in
// Redis, since streams store each field name per entry.
type ClickProducer struct {
	client     *redis.Client // shared Redis connection
	streamName string        // Redis Stream key (e.g., "clicks")
}

// NewClickProducer creates a producer that writes to the given Redis Stream.
// The streamName is typically a constant like "clicks" defined in the
// application config; Redis auto-creates the stream on the first XADD.
func NewClickProducer(client *redis.Client, streamName string) *ClickProducer {
	return &ClickProducer{
		client:     client,
		streamName: streamName,
	}
}

// Publish writes a single ClickEvent to the Redis Stream via XADD. Only
// non-empty optional fields are included in the entry to minimise per-entry
// storage overhead in the stream.
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
	if event.OriginalURL != "" {
		fields["original_url"] = event.OriginalURL
	}
	if event.Referer != "" {
		fields["referer"] = event.Referer
	}
	if event.QueryParams != "" {
		fields["query_params"] = event.QueryParams
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

// PublishBatch writes multiple ClickEvents in a single Redis pipeline round-
// trip. This is useful for bulk-replay or testing scenarios where publishing
// events one-at-a-time would be unnecessarily slow. The pipeline groups all
// XADD commands into one network flush, so N events cost roughly the same
// latency as a single Publish call.
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
		if event.OriginalURL != "" {
			fields["original_url"] = event.OriginalURL
		}
		if event.Referer != "" {
			fields["referer"] = event.Referer
		}
		if event.QueryParams != "" {
			fields["query_params"] = event.QueryParams
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

// StreamInfo returns metadata about the underlying Redis Stream, including its
// length, first and last entries, and the number of consumer groups. This is
// intended for admin/debug endpoints that need to inspect stream health.
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

// StreamLength returns the number of entries in the Redis Stream. It is a
// lightweight alternative to StreamInfo when only the count is needed (e.g.,
// for backpressure checks before publishing more events).
func (p *ClickProducer) StreamLength(ctx context.Context) (int64, error) {
	result := p.client.XLen(ctx, p.streamName)
	return result.Val(), result.Err()
}
