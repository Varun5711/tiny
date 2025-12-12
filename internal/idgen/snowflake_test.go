package idgen

import (
	"fmt"
	"sync"
	"testing"
)

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
					t.Error("generator is nil")
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

	extractedDC := (id >> datacenterIDShift) & maxDatacenterID
	if extractedDC != datacenterID {
		t.Errorf("datacenter ID in ID = %d, want %d", extractedDC, datacenterID)
	}

	extractedWorker := (id >> workerIDShift) & maxWorkerID
	if extractedWorker != workerID {
		t.Errorf("worker ID in ID = %d, want %d", extractedWorker, workerID)
	}

	t.Logf("ID: %d contains datacenter=%d, worker=%d ✓", id, extractedDC, extractedWorker)
}

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

		t.Logf("ID %d → %s (length: %d)", id, shortCode, len(shortCode))
	}
}

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
