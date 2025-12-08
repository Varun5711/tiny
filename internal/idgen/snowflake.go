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
	customEpoch       = 1704067200000 // time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
)

type Generator struct {
	mu sync.Mutex // because multiple goroutines can call Nextid() at same time to hume uske shared state ko protect krna h

	datacenterID int64
	workerID     int64

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

	g := Generator{
		datacenterID: datacenterID,
		workerID:     workerID,
	}

	return &g, nil
}

// NextID generates the next unique Snowflake ID
// This is the CORE method - let me explain each part in detail
func (g *Generator) NextID() (int64, error) {
	// Step 1: LOCK the mutex
	// Why? Because if two goroutines call NextID() at the same time,
	// they might both read sequence=5, increment to 6, and return duplicate IDs!
	// The mutex ensures only ONE goroutine can be in this function at a time
	g.mu.Lock()
	defer g.mu.Unlock() // Unlock when function exits (even if there's an error!)

	// Step 2: Get current timestamp (milliseconds since our custom epoch)
	timestamp := g.currentTimestamp()

	// Step 3: Check for CLOCK MOVING BACKWARDS
	// Example: lastTimestamp=1000, but now timestamp=999
	// This happens if system time is adjusted (NTP sync, manual change, etc.)
	// WHY IS THIS BAD? We might generate duplicate IDs!
	// If we generated ID at timestamp 1000, and now go back to 999,
	// we could generate the SAME ID again!
	if timestamp < g.lastTimestamp {
		return 0, fmt.Errorf("clock moved backwards: refusing to generate ID for %d milliseconds", g.lastTimestamp-timestamp)
	}

	// Step 4: Handle SAME MILLISECOND case
	// If multiple NextID() calls happen in the same millisecond,
	// we increment the sequence counter
	if timestamp == g.lastTimestamp {
		// Increment sequence: 0 → 1 → 2 → ... → 4095
		// The "& maxSequence" keeps it in range:
		// - If sequence=4095, sequence+1=4096
		// - 4096 & maxSequence = 0 (wraps around)
		g.sequence = (g.sequence + 1) & maxSequence

		// If sequence wrapped to 0, we've generated 4096 IDs this millisecond!
		// We MUST wait for next millisecond to avoid ID collision
		if g.sequence == 0 {
			// Busy-wait until clock advances
			timestamp = g.waitForNextMillis(g.lastTimestamp)
		}
	} else {
		// Step 5: NEW MILLISECOND - reset sequence to 0
		// We're in a new millisecond, so we can start counting from 0 again
		g.sequence = 0
	}

	// Step 6: Update lastTimestamp
	g.lastTimestamp = timestamp

	// Step 7: BUILD THE FINAL ID
	// This is where the magic happens! We combine 4 parts into one 64-bit number
	//
	// Let's use an example:
	// timestamp = 1000 (41 bits)
	// datacenterID = 1 (5 bits)
	// workerID = 2 (5 bits)
	// sequence = 5 (12 bits)
	//
	// Step 7a: Position each part using bit shifting
	//
	// timestamp << 22:
	//   Shift timestamp left by 22 bits
	//   This puts it in positions 22-62 of the final ID
	//   Result: ...001000|00000|00000|000000000000
	//
	// datacenterID << 17:
	//   Shift datacenter left by 17 bits
	//   This puts it in positions 17-21
	//   Result: ...000000|00001|00000|000000000000
	//
	// workerID << 12:
	//   Shift worker left by 12 bits
	//   This puts it in positions 12-16
	//   Result: ...000000|00000|00010|000000000000
	//
	// sequence (no shift):
	//   Stays in positions 0-11
	//   Result: ...000000|00000|00000|000000000101
	//
	// Step 7b: Combine them with OR (|)
	//   The OR operation combines all parts without collision
	//   because each part is in a different bit range
	//
	//   Final: ...001000|00001|00010|000000000101
	//
	id := (timestamp << timestampShift) |
		(g.datacenterID << datacenterIDShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

// currentTimestamp returns current time in milliseconds since our custom epoch
//
// Why use a custom epoch instead of Unix epoch (1970)?
// - Unix epoch: time.Now().UnixMilli() gives us a HUGE number (1733689200000+)
// - This wastes bits on "history" we don't care about (1970-2024)
// - Our custom epoch (2024): Subtract 1704067200000 to get smaller numbers
// - This gives us 69 years into the future (2024-2093) before overflow
//
// Example:
// - Current time: 1733689200000 ms (some date in 2024)
// - Custom epoch: 1704067200000 ms (Jan 1, 2024)
// - Result: 29622000000 ms (~343 days since Jan 1, 2024)
//   Much smaller number = more headroom before hitting 41-bit limit!
func (g *Generator) currentTimestamp() int64 {
	return time.Now().UnixMilli() - customEpoch
}

// waitForNextMillis waits until the next millisecond
//
// When is this called?
// - When we've generated 4096 IDs in a single millisecond (sequence overflow)
//
// What does it do?
// - Keeps checking the clock until it advances past lastTimestamp
//
// Why busy-wait instead of time.Sleep(1ms)?
// - time.Sleep() might sleep for MORE than 1ms (OS scheduling)
// - Busy-waiting ensures we get the EXACT next millisecond
// - Downside: Uses CPU while waiting (but this is rare!)
//
// Example flow:
// - lastTimestamp = 1000
// - currentTimestamp() returns 1000 (same!)
// - Loop: timestamp = 1000, keep waiting...
// - Loop: timestamp = 1001, BREAK! Return 1001
func (g *Generator) waitForNextMillis(lastTimestamp int64) int64 {
	timestamp := g.currentTimestamp()

	// Keep looping until we get a new millisecond
	for timestamp <= lastTimestamp {
		timestamp = g.currentTimestamp()
	}

	return timestamp
}

