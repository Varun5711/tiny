package cache

import (
	"container/list"
	"sync"
)

// LRUCache is a thread-safe, fixed-capacity cache that evicts the
// least-recently-used entry when the capacity limit is reached.
//
// It is used as the L1 (in-process) tier of the multi-tier cache. LRU was
// chosen over LFU because URL shortener traffic exhibits temporal locality --
// a link that is "hot right now" (e.g., just shared on social media) matters
// more than a link that accumulated many clicks over months. LRU naturally
// favours recent access, so it keeps the current working set warm without
// needing the extra bookkeeping (frequency counters, aging) that LFU requires.
//
// Implementation: a hash map for O(1) key lookup combined with a doubly-linked
// list (container/list) for O(1) promotion and eviction. Every access moves
// the entry to the front of the list; eviction removes from the back.
//
// Thread safety is provided by a sync.RWMutex. Get uses a full Lock (not
// RLock) because it mutates the list order on every hit.
type LRUCache struct {
	capacity int                      // maximum number of entries before eviction
	cache    map[string]*list.Element // O(1) key -> list node lookup
	lruList  *list.List               // doubly-linked list ordered by recency (front = most recent)
	mu       sync.RWMutex             // guards concurrent access to cache and lruList
}

// entry is the value stored inside each list.Element. It pairs the key with
// the cached value so that evict() can delete the map entry in O(1) without
// a reverse lookup.
type entry struct {
	key   string
	value interface{}
}

// NewLRUCache creates an empty LRU cache that will hold at most capacity
// entries. A capacity of 0 means every Set is immediately followed by an
// eviction, effectively making the cache a no-op (useful in tests).
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// Get retrieves the value associated with key and marks it as most-recently
// used by moving it to the front of the LRU list. Returns (value, true) on a
// hit, or (nil, false) on a miss.
//
// Note: Get takes a write lock (not a read lock) because the MoveToFront call
// mutates the linked list. Under high read concurrency a sharded design would
// reduce contention, but for the expected hot-key count (~thousands) the
// single-lock approach is simpler and sufficient.
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.lruList.MoveToFront(elem)
		return elem.Value.(*entry).value, true
	}
	return nil, false
}

// Set inserts or updates a key-value pair. If the key already exists its value
// is updated and the entry is promoted to the front. If the key is new and the
// cache is at capacity, the least-recently-used entry (back of the list) is
// evicted first to make room.
func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.lruList.MoveToFront(elem)
		elem.Value.(*entry).value = value
		return
	}

	elem := c.lruList.PushFront(&entry{key, value})
	c.cache[key] = elem

	if c.lruList.Len() > c.capacity {
		c.evict()
	}
}

// Delete removes a key from the cache. It is a no-op if the key does not
// exist, which is safe for callers that invalidate without checking presence.
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.lruList.Remove(elem)
		delete(c.cache, key)
	}
}

// evict removes the least-recently-used entry (the back of the list). It must
// be called while c.mu is already held. The entry stores its own key so the
// corresponding map entry can be deleted without a reverse lookup.
func (c *LRUCache) evict() {
	elem := c.lruList.Back()
	if elem != nil {
		c.lruList.Remove(elem)
		delete(c.cache, elem.Value.(*entry).key)
	}
}

// Len returns the number of entries currently in the cache. It is safe for
// concurrent use.
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Clear removes all entries from the cache by replacing the internal map and
// list with fresh instances. The old structures become eligible for garbage
// collection immediately.
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
}
