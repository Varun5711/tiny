package idgen

import (
	"fmt"
	"sync"
	"testing"
)

// TestNewGenerator validates the constructor's input bounds checking. The
// datacenter and worker IDs are each 5 bits, so valid values are 0-31.
// The test covers zero, maximum, mid-range, negative, and out-of-range inputs.
func TestNewGenerator(t *testing.T) {
	tests := []struct {
		name         string
		datacenterID int64
		workerID     int64
		shouldError  bool
	}{
		{"valid IDs", 0, 0, false},
		{"valid max IDs", 31, 31, false},
		{"valid mid IDs", 15, 15, false},
		{"invalid datacenter negative", -1, 0, true},
		{"invalid datacenter too large", 32, 0, true},
		{"invalid worker negative", 0, -1, true},
		{"invalid worker too large", 0, 32, true},
		{"both invalid", 100, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewGenerator(tt.datacenterID, tt.workerID)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for datacenter=%d, worker=%d, got nil", tt.datacenterID, tt.workerID)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if gen == nil {
					t.Fatal("generator is nil")
				}
				if gen.datacenterID != tt.datacenterID {
					t.Errorf("datacenterID = %d, want %d", gen.datacenterID, tt.datacenterID)
				}
				if gen.workerID != tt.workerID {
					t.Errorf("workerID = %d, want %d", gen.workerID, tt.workerID)
				}
			}
		})
	}
}

// TestNextID verifies basic ID generation: IDs must be non-zero, unique, and
// monotonically increasing (since the timestamp component dominates ordering).
func TestNextID(t *testing.T) {
	gen, err := NewGenerator(1, 1)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	id1, err := gen.NextID()
	if err != nil {
		t.Fatalf("NextID() error: %v", err)
	}

	if id1 == 0 {
		t.Error("generated ID is 0")
	}

	id2, err := gen.NextID()
	if err != nil {
		t.Fatalf("NextID() error: %v", err)
	}

	if id1 == id2 {
		t.Errorf("duplicate IDs: %d == %d", id1, id2)
	}

	if id2 <= id1 {
		t.Errorf("IDs not ordered: %d should be > %d", id2, id1)
	}

	t.Logf("Generated IDs: %d, %d", id1, id2)
}

// TestUniqueIDs generates 10,000 IDs sequentially from a single generator
// and verifies that no duplicates are produced. This exercises the sequence
// counter within a single millisecond window.
func TestUniqueIDs(t *testing.T) {
	gen, err := NewGenerator(1, 1)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	count := 10000
	ids := make(map[int64]bool, count)

	for i := 0; i < count; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("NextID() error at iteration %d: %v", i, err)
		}

		if ids[id] {
			t.Fatalf("duplicate ID found: %d", id)
		}
		ids[id] = true
	}

	t.Logf("Successfully generated %d unique IDs", count)
}

// TestConcurrentGeneration validates thread safety by generating IDs from
// 10 goroutines simultaneously. This is the most critical test for a
// Snowflake generator, as the mutex must correctly serialize access to the
// sequence counter and lastTimestamp fields across concurrent callers.
func TestConcurrentGeneration(t *testing.T) {
	gen, err := NewGenerator(1, 1)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	numGoroutines := 10
	idsPerGoroutine := 1000

	var mu sync.Mutex
	ids := make(map[int64]bool, numGoroutines*idsPerGoroutine)

	var wg sync.WaitGroup

	errors := make(chan error, numGoroutines*idsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < idsPerGoroutine; j++ {
				id, err := gen.NextID()
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: %v", goroutineID, err)
					return
				}

				mu.Lock()
				if ids[id] {
					errors <- fmt.Errorf("duplicate ID: %d in goroutine %d", id, goroutineID)
					mu.Unlock()
					return
				}
				ids[id] = true
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}

	expectedCount := numGoroutines * idsPerGoroutine
	if len(ids) != expectedCount {
		t.Errorf("expected %d unique IDs, got %d", expectedCount, len(ids))
	}

	t.Logf("Successfully generated %d unique IDs across %d goroutines", len(ids), numGoroutines)
}

// TestIDStructure verifies that the datacenter and worker IDs are correctly
// embedded in the generated Snowflake ID by extracting them with bit-shifting
// and masking. This ensures the bit layout matches the documented format:
// [41-bit timestamp][5-bit datacenter][5-bit worker][12-bit sequence].
func TestIDStructure(t *testing.T) {
	datacenterID := int64(5)
	workerID := int64(10)

	gen, err := NewGenerator(datacenterID, workerID)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	id, err := gen.NextID()
	if err != nil {
		t.Fatalf("NextID() error: %v", err)
	}

	// Extract the datacenter ID from bits 17-21 by right-shifting past the
	// worker and sequence bits, then masking to 5 bits.
	extractedDC := (id >> datacenterIDShift) & maxDatacenterID
	if extractedDC != datacenterID {
		t.Errorf("datacenter ID in ID = %d, want %d", extractedDC, datacenterID)
	}

	// Extract the worker ID from bits 12-16 by right-shifting past the
	// sequence bits, then masking to 5 bits.
	extractedWorker := (id >> workerIDShift) & maxWorkerID
	if extractedWorker != workerID {
		t.Errorf("worker ID in ID = %d, want %d", extractedWorker, workerID)
	}

	t.Logf("ID: %d contains datacenter=%d, worker=%d", id, extractedDC, extractedWorker)
}

// TestBase62Conversion performs an end-to-end integration test of the full
// ID pipeline: Snowflake generation -> Base62 encoding -> Base62 decoding.
// It verifies that the round trip is lossless and that the resulting short
// codes are compact (at most 12 characters for any valid Snowflake ID).
func TestBase62Conversion(t *testing.T) {
	gen, err := NewGenerator(1, 1)
	if err != nil {
		t.Fatalf("failed to create generator: %v", err)
	}

	for i := 0; i < 10; i++ {
		id, err := gen.NextID()
		if err != nil {
			t.Fatalf("NextID() error: %v", err)
		}

		shortCode := Encode(id)

		decoded, err := Decode(shortCode)
		if err != nil {
			t.Fatalf("Decode error: %v", err)
		}

		if decoded != id {
			t.Errorf("round trip failed: %d -> %s -> %d", id, shortCode, decoded)
		}

		if len(shortCode) > 12 {
			t.Errorf("short code too long: %s (%d chars)", shortCode, len(shortCode))
		}

		t.Logf("ID %d -> %s (length: %d)", id, shortCode, len(shortCode))
	}
}

// BenchmarkNextID measures single-threaded ID generation throughput.
func BenchmarkNextID(b *testing.B) {
	gen, _ := NewGenerator(1, 1)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := gen.NextID()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNextIDParallel measures ID generation throughput under contention
// from multiple goroutines, which is the realistic production scenario.
// This benchmark reveals the cost of mutex contention on the generator.
func BenchmarkNextIDParallel(b *testing.B) {
	gen, _ := NewGenerator(1, 1)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := gen.NextID()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
