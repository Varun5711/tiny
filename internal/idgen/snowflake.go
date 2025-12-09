package idgen

import (
	"fmt"
	"sync"
	"time"
)

const (
	workerIDBits      = 5
	datacenterIDBits  = 5
	sequenceBits      = 12
	maxWorkerID       = -1 ^ (-1 << workerIDBits)
	maxDatacenterID   = -1 ^ (-1 << datacenterIDBits)
	maxSequence       = -1 ^ (-1 << sequenceBits)
	workerIDShift     = sequenceBits
	datacenterIDShift = sequenceBits + workerIDBits
	timestampShift    = sequenceBits + workerIDBits + datacenterIDBits
	customEpoch       = 1704067200000
)

type Generator struct {
	mu            sync.Mutex
	datacenterID  int64
	workerID      int64
	sequence      int64
	lastTimestamp int64
}

func NewGenerator(datacenterID, workerID int64) (*Generator, error) {
	if datacenterID < 0 || datacenterID > maxDatacenterID {
		return nil, fmt.Errorf("datacenter ID must be between 0 and %d", maxDatacenterID)
	}

	if workerID < 0 || workerID > maxWorkerID {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", workerID)
	}

	return &Generator{
		datacenterID: datacenterID,
		workerID:     workerID,
	}, nil
}

func (g *Generator) NextID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	timestamp := g.currentTimestamp()

	if timestamp < g.lastTimestamp {
		return 0, fmt.Errorf("clock moved backwards: refusing to generate ID for %d milliseconds", g.lastTimestamp-timestamp)
	}

	if timestamp == g.lastTimestamp {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			timestamp = g.waitForNextMillis(g.lastTimestamp)
		}
	} else {
		g.sequence = 0
	}

	g.lastTimestamp = timestamp

	id := (timestamp << timestampShift) |
		(g.datacenterID << datacenterIDShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

func (g *Generator) currentTimestamp() int64 {
	return time.Now().UnixMilli() - customEpoch
}

func (g *Generator) waitForNextMillis(lastTimestamp int64) int64 {
	timestamp := g.currentTimestamp()
	for timestamp <= lastTimestamp {
		timestamp = g.currentTimestamp()
	}
	return timestamp
}
