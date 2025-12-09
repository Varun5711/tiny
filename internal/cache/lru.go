package cache

import (
	"container/list"
	"sync"
)

type LRUCache struct {
	capacity int
	cache    map[string]*list.Element
	lruList  *list.List
	mu       sync.RWMutex
}

type entry struct {
	key   string
	value interface{}
}

func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.lruList.MoveToFront(elem)
		return elem.Value.(*entry).value, true
	}
	return nil, false
}

func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key exists, update it and move to front
	if elem, found := c.cache[key]; found {
		c.lruList.MoveToFront(elem)
		elem.Value.(*entry).value = value
		return
	}

	// Add new item to front (most recently used)
	elem := c.lruList.PushFront(&entry{key, value})
	c.cache[key] = elem

	// If over capacity, remove least recently used (back of list)
	if c.lruList.Len() > c.capacity {
		c.evict()
	}
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.cache[key]; found {
		c.lruList.Remove(elem)
		delete(c.cache, key)
	}
}

func (c *LRUCache) evict() {
	elem := c.lruList.Back()
	if elem != nil {
		c.lruList.Remove(elem)
		delete(c.cache, elem.Value.(*entry).key)
	}
}

func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
}
