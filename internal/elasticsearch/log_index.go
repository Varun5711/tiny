package elasticsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// LogEntry represents a single structured log record destined for Elasticsearch.
// The Fields map captures any extra key-value pairs from the original log line
// (e.g. request_id, latency_ms) that are not part of the standard envelope.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogSyncer buffers structured log entries and ships them to Elasticsearch in
// batches via the Bulk API. It implements both io.Writer and zapcore.WriteSyncer,
// so it can be plugged directly into a zap logger as a write destination.
//
// Batching is critical for log shipping because writing each log line as an
// individual ES request would overwhelm the cluster with HTTP overhead. Instead,
// LogSyncer accumulates entries in an in-memory buffer and flushes them either
// when the buffer reaches flushSize or when the flushTicker fires, whichever
// comes first. This provides bounded latency (logs appear in ES within the
// flush interval) while keeping throughput high under load.
type LogSyncer struct {
	client      *Client
	service     string
	buffer      []LogEntry
	mu          sync.Mutex
	flushSize   int
	flushTicker *time.Ticker
	done        chan struct{}
}

// NewLogSyncer creates a LogSyncer that batches log entries and flushes them
// to Elasticsearch on a timer or when the buffer is full. The background
// goroutine started here ticks at flushInterval and calls Flush, ensuring
// logs are shipped even during low-traffic periods. The caller must call
// Close to stop the ticker and drain the buffer on shutdown.
func NewLogSyncer(client *Client, service string, flushInterval time.Duration, flushSize int) *LogSyncer {
	ls := &LogSyncer{
		client:      client,
		service:     service,
		buffer:      make([]LogEntry, 0, flushSize),
		flushSize:   flushSize,
		flushTicker: time.NewTicker(flushInterval),
		done:        make(chan struct{}),
	}

	go ls.run()
	return ls
}

// run is the background flush loop. It blocks on the ticker channel and
// flushes buffered logs on each tick. The loop exits when the done channel
// is closed by Close.
func (ls *LogSyncer) run() {
	for {
		select {
		case <-ls.flushTicker.C:
			ls.Flush()
		case <-ls.done:
			return
		}
	}
}

// logIndex returns a daily-rotating index name in the format:
//
//	<prefix>-logs-<service>-YYYY-MM-DD
//
// Daily rotation keeps individual indices small and makes retention easy --
// old indices can be deleted by date using Elasticsearch ILM (Index Lifecycle
// Management) or a simple cron job, without needing to reindex.
func (ls *LogSyncer) logIndex() string {
	return fmt.Sprintf("%s-logs-%s-%s",
		ls.client.indexPrefix,
		ls.service,
		time.Now().Format("2006-01-02"),
	)
}

// Write implements io.Writer. It parses incoming bytes as a JSON log line
// (the format produced by zap's JSON encoder), extracts known fields
// (level, msg, timestamp), and collects everything else into the Fields map.
//
// Write always returns len(p), nil -- even if JSON parsing fails -- because
// returning an error from a logger's writer would cause the logger itself to
// fail, which is worse than silently dropping a malformed log line. The
// successfully parsed entry is appended to the buffer under a mutex, and
// if the buffer has reached flushSize, Flush is triggered immediately to
// prevent unbounded memory growth under high log volume.
func (ls *LogSyncer) Write(p []byte) (int, error) {
	var entry map[string]interface{}
	if err := json.Unmarshal(p, &entry); err != nil {
		// Silently drop unparseable lines to avoid crashing the logger.
		return len(p), nil
	}

	logEntry := LogEntry{
		Timestamp: time.Now(),
		Service:   ls.service,
	}

	// Extract well-known fields from the JSON log line into the structured
	// LogEntry envelope. Any remaining fields become metadata in Fields.
	if level, ok := entry["level"].(string); ok {
		logEntry.Level = level
	}
	if msg, ok := entry["msg"].(string); ok {
		logEntry.Message = msg
	}
	if ts, ok := entry["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			logEntry.Timestamp = parsed
		}
	}

	// Collect all non-standard fields (e.g. request_id, latency, error)
	// into the Fields map for ad-hoc querying in Kibana/ES.
	fields := make(map[string]interface{})
	for k, v := range entry {
		if k != "level" && k != "msg" && k != "timestamp" && k != "service" {
			fields[k] = v
		}
	}
	if len(fields) > 0 {
		logEntry.Fields = fields
	}

	ls.mu.Lock()
	ls.buffer = append(ls.buffer, logEntry)
	shouldFlush := len(ls.buffer) >= ls.flushSize
	ls.mu.Unlock()

	// Trigger an immediate flush if the buffer is full. This is done
	// outside the lock to avoid holding the mutex during the ES call.
	if shouldFlush {
		ls.Flush()
	}

	return len(p), nil
}

// Flush drains the internal buffer and sends all accumulated log entries to
// Elasticsearch in a single Bulk API call. The buffer is swapped under the
// mutex so that new writes can continue while the network call is in flight.
//
// The Bulk payload uses NDJSON format (alternating action/metadata and
// document lines, each terminated by \n). No document IDs are specified in
// the action lines because log entries are append-only -- ES auto-generates
// IDs, which is faster than explicit ID assignment for write-heavy workloads.
//
// Errors from the Bulk call are intentionally swallowed because log shipping
// is best-effort: a transient ES failure should not crash the application or
// cause log loss in the primary output (stdout/file).
func (ls *LogSyncer) Flush() {
	ls.mu.Lock()
	if len(ls.buffer) == 0 {
		ls.mu.Unlock()
		return
	}
	// Swap the buffer so writes can continue concurrently with the flush.
	entries := ls.buffer
	ls.buffer = make([]LogEntry, 0, ls.flushSize)
	ls.mu.Unlock()

	index := ls.logIndex()

	// Build the NDJSON Bulk payload: pairs of action-line + document-line.
	var buf bytes.Buffer
	for _, entry := range entries {
		// Action line: "index" into the daily log index, no explicit _id
		// (auto-generated IDs are faster for append-only workloads).
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
			},
		}
		metaLine, _ := json.Marshal(meta)
		buf.Write(metaLine)
		buf.WriteByte('\n')

		dataLine, _ := json.Marshal(entry)
		buf.Write(dataLine)
		buf.WriteByte('\n')
	}

	// Fire-and-forget: log shipping errors are intentionally ignored so
	// a transient ES outage does not disrupt the application.
	_, _ = ls.client.es.Bulk(bytes.NewReader(buf.Bytes()))
}

// Sync implements zapcore.WriteSyncer. It flushes the buffer to ensure all
// buffered log entries are sent to Elasticsearch, then returns nil. This is
// called by zap on logger.Sync() (typically at application shutdown).
func (ls *LogSyncer) Sync() error {
	ls.Flush()
	return nil
}

// Close stops the background flush ticker, shuts down the flush goroutine,
// and performs a final flush to drain any remaining buffered entries. Callers
// should invoke Close during graceful shutdown to avoid losing recent logs.
func (ls *LogSyncer) Close() {
	ls.flushTicker.Stop()
	close(ls.done)
	ls.Flush()
}
