// Package events defines the analytics event models and producers for the URL
// shortener.
//
// When a user clicks a short link, the redirect handler publishes a ClickEvent
// to a Redis Stream. A separate consumer (not in this package) reads the stream
// and writes aggregated analytics to persistent storage. This decouples the
// latency-sensitive redirect path from the slower analytics write path --
// the redirect returns immediately after the XADD, and the consumer processes
// events at its own pace.
//
// Redis Streams were chosen over a traditional message broker because the
// project already depends on Redis for caching and locking, so no additional
// infrastructure is required. Streams also provide built-in consumer groups,
// delivery guarantees, and backpressure via XLEN.
package events

// ClickEvent represents a single redirect ("click") that should be recorded
// for analytics. Only ShortCode and Timestamp are mandatory; the remaining
// fields are populated on a best-effort basis from the HTTP request headers.
//
// Fields are stored as flat key-value pairs in the Redis Stream entry rather
// than as a single JSON blob. This makes it possible to filter or inspect
// events with native Redis commands (e.g., XRANGE + field selectors) without
// deserializing.
type ClickEvent struct {
	ShortCode   string // the short code that was resolved (e.g., "abc123")
	Timestamp   int64  // Unix epoch millis when the redirect occurred
	IP          string // client IP, empty if not available or redacted for privacy
	UserAgent   string // User-Agent header, useful for bot detection
	OriginalURL string // the long URL the short code resolved to
	Referer     string // HTTP Referer header, indicates where the click came from
	QueryParams string // raw query string forwarded from the short link
}
