package cache

import (
	"sync"
	"testing"
)

// TestNewLRUCache verifies that a freshly created cache has the requested
// capacity and starts empty.
func TestNewLRUCache(t *testing.T) {
	cache := NewLRUCache(10)

	if cache == nil {
		t.Fatal("expected cache to be created")
	}
	if cache.capacity != 10 {
		t.Errorf("expected capacity 10, got %d", cache.capacity)
	}
	if cache.Len() != 0 {
		t.Errorf("expected empty cache, got length %d", cache.Len())
	}
}

// TestLRUCache_SetAndGet confirms the basic store-then-retrieve path works.
func TestLRUCache_SetAndGet(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1")

	value, found := cache.Get("key1")
	if !found {
		t.Error("expected to find key1")
	}
	if value != "value1" {
		t.Errorf("expected 'value1', got '%v'", value)
	}
}

// TestLRUCache_GetNotFound ensures a miss returns (nil, false) rather than
// panicking or returning a zero value.
func TestLRUCache_GetNotFound(t *testing.T) {
	cache := NewLRUCache(10)

	value, found := cache.Get("nonexistent")
	if found {
		t.Error("expected not to find nonexistent key")
	}
	if value != nil {
		t.Errorf("expected nil value, got '%v'", value)
	}
}

// TestLRUCache_UpdateExisting verifies that setting an existing key updates the
// value in-place without increasing the cache length.
func TestLRUCache_UpdateExisting(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1")
	cache.Set("key1", "value2")

	value, found := cache.Get("key1")
	if !found {
		t.Error("expected to find key1")
	}
	if value != "value2" {
		t.Errorf("expected 'value2', got '%v'", value)
	}
	if cache.Len() != 1 {
		t.Errorf("expected length 1, got %d", cache.Len())
	}
}

// TestLRUCache_Eviction checks that inserting beyond capacity evicts the
// least-recently-used entry (the oldest untouched key).
func TestLRUCache_Eviction(t *testing.T) {
	cache := NewLRUCache(3)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	if cache.Len() != 3 {
		t.Errorf("expected length 3, got %d", cache.Len())
	}

	cache.Set("key4", "value4")

	if cache.Len() != 3 {
		t.Errorf("expected length 3 after eviction, got %d", cache.Len())
	}

	_, found := cache.Get("key1")
	if found {
		t.Error("expected key1 to be evicted (LRU)")
	}

	_, found = cache.Get("key4")
	if !found {
		t.Error("expected key4 to be present")
	}
}

// TestLRUCache_LRUOrder verifies that a Get promotes an entry, protecting it
// from eviction. key1 is accessed after key3, so key2 becomes the LRU victim
// when key4 is inserted.
func TestLRUCache_LRUOrder(t *testing.T) {
	cache := NewLRUCache(3)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	cache.Get("key1")

	cache.Set("key4", "value4")

	_, found := cache.Get("key1")
	if !found {
		t.Error("expected key1 to still be present (recently accessed)")
	}

	_, found = cache.Get("key2")
	if found {
		t.Error("expected key2 to be evicted (LRU)")
	}
}

// TestLRUCache_Delete ensures that explicit deletion removes only the targeted
// key and leaves other entries intact.
func TestLRUCache_Delete(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	cache.Delete("key1")

	_, found := cache.Get("key1")
	if found {
		t.Error("expected key1 to be deleted")
	}

	_, found = cache.Get("key2")
	if !found {
		t.Error("expected key2 to still be present")
	}

	if cache.Len() != 1 {
		t.Errorf("expected length 1, got %d", cache.Len())
	}
}

// TestLRUCache_DeleteNonExistent confirms that deleting a key that was never
// inserted is a safe no-op.
func TestLRUCache_DeleteNonExistent(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Delete("nonexistent")

	if cache.Len() != 0 {
		t.Errorf("expected length 0, got %d", cache.Len())
	}
}

// TestLRUCache_Clear verifies that Clear removes all entries and resets the
// length to zero.
func TestLRUCache_Clear(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", cache.Len())
	}

	_, found := cache.Get("key1")
	if found {
		t.Error("expected cache to be empty after clear")
	}
}

// TestLRUCache_DifferentValueTypes exercises the interface{} value slot with
// strings, ints, and structs to confirm type-agnostic storage.
func TestLRUCache_DifferentValueTypes(t *testing.T) {
	cache := NewLRUCache(10)

	cache.Set("string", "value")
	cache.Set("int", 42)
	cache.Set("struct", struct{ Name string }{"test"})

	v, _ := cache.Get("string")
	if v != "value" {
		t.Errorf("expected 'value', got '%v'", v)
	}

	v, _ = cache.Get("int")
	if v != 42 {
		t.Errorf("expected 42, got '%v'", v)
	}

	v, _ = cache.Get("struct")
	s, ok := v.(struct{ Name string })
	if !ok || s.Name != "test" {
		t.Errorf("expected struct with Name 'test', got '%v'", v)
	}
}

// TestLRUCache_Concurrent stress-tests the cache under concurrent access. 100
// goroutines each perform 100 Set/Get cycles. The test passes if no race
// detector violations or panics occur (run with -race to verify).
func TestLRUCache_Concurrent(t *testing.T) {
	cache := NewLRUCache(100)

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id+j)%26))
				cache.Set(key, id*numOperations+j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()
}

// TestLRUCache_ZeroCapacity confirms the edge case: a cache with capacity 0
// evicts every entry immediately after insertion, keeping length at 0.
func TestLRUCache_ZeroCapacity(t *testing.T) {
	cache := NewLRUCache(0)

	cache.Set("key1", "value1")

	if cache.Len() != 0 {
		t.Errorf("expected length 0 for zero capacity cache, got %d", cache.Len())
	}
}
