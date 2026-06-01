// Package idgen implements distributed unique ID generation and encoding for the
// Tiny URL shortener. It combines a Snowflake-inspired ID generator with Base62
// encoding to produce short, URL-safe, globally unique identifiers without
// requiring coordination between nodes.
//
// The Snowflake generator produces 63-bit positive integers that are roughly
// time-ordered, unique across a cluster of up to 32 datacenters x 32 workers,
// and capable of generating up to 4096 IDs per millisecond per worker. The
// Base62 encoder then compresses these integers into compact alphanumeric
// strings suitable for use as short URL codes.
package idgen

import (
	"fmt"
	"sync"
	"time"
)

const (
	// workerIDBits is the number of bits allocated to the worker (machine) ID
	// within a datacenter. 5 bits allows 2^5 = 32 distinct workers (0-31)
	// per datacenter.
	workerIDBits = 5

	// datacenterIDBits is the number of bits allocated to the datacenter ID.
	// 5 bits allows 2^5 = 32 distinct datacenters (0-31), giving a total
	// cluster capacity of 32 * 32 = 1024 unique generator instances.
	datacenterIDBits = 5

	// sequenceBits is the number of bits allocated to the per-millisecond
	// sequence counter. 12 bits allows 2^12 = 4096 unique IDs per millisecond
	// per worker before the generator must wait for the next millisecond tick.
	sequenceBits = 12

	// maxWorkerID is the maximum valid worker ID (31). Computed as a bitmask:
	// -1 ^ (-1 << 5) flips the lower 5 bits to 1, yielding 0b11111 = 31.
	maxWorkerID = -1 ^ (-1 << workerIDBits)

	// maxDatacenterID is the maximum valid datacenter ID (31), computed
	// identically to maxWorkerID using the datacenter bit width.
	maxDatacenterID = -1 ^ (-1 << datacenterIDBits)

	// maxSequence is the maximum sequence number (4095) within a single
	// millisecond. When the sequence exceeds this value (wraps to 0), the
	// generator spin-waits for the next millisecond to avoid collisions.
	maxSequence = -1 ^ (-1 << sequenceBits)

	// workerIDShift is the number of bits to left-shift the worker ID so it
	// sits immediately above the sequence bits in the final 64-bit ID.
	// Layout: [unused 1-bit sign][41-bit timestamp][5-bit DC][5-bit worker][12-bit seq]
	workerIDShift = sequenceBits

	// datacenterIDShift is the number of bits to left-shift the datacenter ID
	// so it sits above both the worker ID and sequence bits.
	datacenterIDShift = sequenceBits + workerIDBits

	// timestampShift is the number of bits to left-shift the timestamp so it
	// occupies the highest 41 bits (below the sign bit) of the 64-bit ID.
	// This ensures IDs are roughly time-ordered: higher timestamps always
	// produce numerically larger IDs regardless of datacenter/worker/sequence.
	timestampShift = sequenceBits + workerIDBits + datacenterIDBits

	// customEpoch is the custom epoch in milliseconds since Unix epoch,
	// corresponding to 2024-01-01 00:00:00 UTC. Using a custom epoch
	// (rather than the Unix epoch of 1970) maximizes the usable range of the
	// 41-bit timestamp field: 2^41 milliseconds is ~69 years, so IDs will
	// not overflow until approximately 2093.
	customEpoch = 1704067200000
)

// Generator produces unique, time-ordered 63-bit Snowflake IDs. Each ID
// encodes a millisecond timestamp, a datacenter identifier, a worker
// identifier, and a per-millisecond sequence number.
//
// The 63-bit ID layout (MSB to LSB):
//
//	[1-bit unused (sign=0)][41-bit timestamp][5-bit datacenterID][5-bit workerID][12-bit sequence]
//
// Generator is safe for concurrent use; the mutex serializes access to the
// mutable sequence counter and last-timestamp tracker.
type Generator struct {
	// mu protects sequence and lastTimestamp from concurrent access.
	// Every call to NextID acquires this lock to ensure the sequence counter
	// increments atomically with the timestamp check.
	mu sync.Mutex

	// datacenterID identifies which datacenter this generator belongs to (0-31).
	datacenterID int64

	// workerID identifies which machine within the datacenter runs this
	// generator (0-31).
	workerID int64

	// sequence is the monotonically increasing counter within a single
	// millisecond. It resets to 0 when the millisecond advances.
	sequence int64

	// lastTimestamp records the most recent millisecond at which an ID was
	// generated. Used to detect clock drift (backward movement) and to
	// determine whether the sequence counter should increment or reset.
	lastTimestamp int64
}

// NewGenerator creates a Snowflake ID generator for the given datacenter and
// worker. Both IDs must be in the range [0, 31]; an error is returned if
// either is out of range.
//
// In a typical deployment each Kubernetes pod or VM receives a unique
// (datacenterID, workerID) pair via environment variables, ensuring no two
// generators in the cluster can produce the same ID.
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

// NextID generates the next unique Snowflake ID. It is safe for concurrent
// use from multiple goroutines.
//
// The method acquires a mutex, reads the current millisecond timestamp
// (relative to the custom epoch), and applies the following logic:
//
//  1. If the clock has moved backward (NTP correction, VM migration, etc.),
//     return an error rather than risk generating duplicate IDs.
//  2. If the timestamp matches the previous call, increment the 12-bit
//     sequence counter. If the sequence overflows (wraps past 4095),
//     spin-wait until the next millisecond to guarantee uniqueness.
//  3. If the timestamp has advanced, reset the sequence to 0.
//
// The final ID is assembled by OR-ing the shifted timestamp, datacenter ID,
// worker ID, and sequence into a single int64.
func (g *Generator) NextID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	timestamp := g.currentTimestamp()

	// Clock drift protection: if system time moved backward (e.g., NTP
	// adjustment), refuse to generate IDs to prevent duplicates.
	if timestamp < g.lastTimestamp {
		return 0, fmt.Errorf("clock moved backwards: refusing to generate ID for %d milliseconds", g.lastTimestamp-timestamp)
	}

	if timestamp == g.lastTimestamp {
		// Same millisecond as the last ID: increment the sequence counter.
		// The bitwise AND with maxSequence (0xFFF) wraps the counter back
		// to 0 when it exceeds 4095, signaling that this millisecond's
		// capacity is exhausted.
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// All 4096 sequence slots for this millisecond are used.
			// Spin-wait until the clock advances to the next millisecond.
			timestamp = g.waitForNextMillis(g.lastTimestamp)
		}
	} else {
		// New millisecond: reset the sequence counter to 0.
		g.sequence = 0
	}

	g.lastTimestamp = timestamp

	// Assemble the 63-bit ID by shifting each component into its designated
	// bit range and combining with bitwise OR:
	//   Bits 22-62: timestamp (41 bits, shifted left by 22)
	//   Bits 17-21: datacenterID (5 bits, shifted left by 17)
	//   Bits 12-16: workerID (5 bits, shifted left by 12)
	//   Bits  0-11: sequence (12 bits, no shift)
	id := (timestamp << timestampShift) |
		(g.datacenterID << datacenterIDShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

// currentTimestamp returns the number of milliseconds elapsed since the
// custom epoch (2024-01-01 00:00:00 UTC). Subtracting the custom epoch
// ensures the 41-bit field is used efficiently, starting near zero for
// recent timestamps rather than carrying decades of unused range.
func (g *Generator) currentTimestamp() int64 {
	return time.Now().UnixMilli() - customEpoch
}

// waitForNextMillis busy-waits until the system clock advances past
// lastTimestamp. This is called when the 12-bit sequence counter overflows
// within a single millisecond (i.e., more than 4096 IDs were requested in
// 1 ms). The spin-wait is acceptable because the maximum wait is under 1 ms
// and this situation is rare in practice.
func (g *Generator) waitForNextMillis(lastTimestamp int64) int64 {
	timestamp := g.currentTimestamp()
	for timestamp <= lastTimestamp {
		timestamp = g.currentTimestamp()
	}
	return timestamp
}
