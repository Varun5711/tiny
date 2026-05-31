package elasticsearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

type LogSyncer struct {
	client      *Client
	service     string
	buffer      []LogEntry
	mu          sync.Mutex
	flushSize   int
	flushTicker *time.Ticker
	done        chan struct{}
}

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

func (ls *LogSyncer) logIndex() string {
	return fmt.Sprintf("%s-logs-%s-%s",
		ls.client.indexPrefix,
		ls.service,
		time.Now().Format("2006-01-02"),
	)
}

func (ls *LogSyncer) Write(p []byte) (int, error) {
	var entry map[string]interface{}
	if err := json.Unmarshal(p, &entry); err != nil {
		return len(p), nil
	}

	logEntry := LogEntry{
		Timestamp: time.Now(),
		Service:   ls.service,
	}

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

	if shouldFlush {
		ls.Flush()
	}

	return len(p), nil
}

func (ls *LogSyncer) Flush() {
	ls.mu.Lock()
	if len(ls.buffer) == 0 {
		ls.mu.Unlock()
		return
	}
	entries := ls.buffer
	ls.buffer = make([]LogEntry, 0, ls.flushSize)
	ls.mu.Unlock()

	index := ls.logIndex()

	var buf bytes.Buffer
	for _, entry := range entries {
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

	ls.client.es.Bulk(bytes.NewReader(buf.Bytes()))
}

func (ls *LogSyncer) Sync() error {
	ls.Flush()
	return nil
}

func (ls *LogSyncer) Close() {
	ls.flushTicker.Stop()
	close(ls.done)
	ls.Flush()
}
