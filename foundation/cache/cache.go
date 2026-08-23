package cache

import (
	"sync"
	"time"
)

const defaultCapacity = 10_000

type LRUCache[K comparable, V any] struct {
	cacheMap map[K]*cacheNode[K, V]
	capacity int
	head     *cacheNode[K, V]
	tail     *cacheNode[K, V]
	ttl      time.Duration
	mu       *sync.Mutex
}

type cacheNode[K comparable, V any] struct {
	key       K
	value     V
	createdAt time.Time
	prev      *cacheNode[K, V]
	next      *cacheNode[K, V]
}

type option[K comparable, V any] func(*LRUCache[K, V])

func WithCapacity[K comparable, V any](capacity int) option[K, V] {
	return option[K, V](func(l *LRUCache[K, V]) {
		l.capacity = capacity
	})
}

func WithTTL[K comparable, V any](ttl time.Duration) option[K, V] {
	return option[K, V](func(l *LRUCache[K, V]) {
		l.ttl = ttl
	})
}

func NewLRU[K comparable, V any](opts ...option[K, V]) *LRUCache[K, V] {
	lruCache := new(LRUCache[K, V])
	lruCache.capacity = defaultCapacity
	lruCache.cacheMap = make(map[K]*cacheNode[K, V])
	lruCache.mu = &sync.Mutex{}
	for _, opt := range opts {
		opt(lruCache)
	}
	return lruCache
}

func (c *LRUCache[K, V]) Get(key K) (value V, ok bool) {
	var zero V
	var zeroTTL time.Duration
	c.mu.Lock()
	defer c.mu.Unlock()
	node, ok := c.cacheMap[key]
	if !ok {
		return zero, false
	}

	c.delete(key)
	if c.ttl != zeroTTL && node.createdAt.Add(c.ttl).Before(time.Now()) {
		return zero, false
	}
	node.createdAt = time.Now()
	c.insert(node)
	return node.value, true
}

func (c *LRUCache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.cacheMap[key]
	if len(c.cacheMap) == c.capacity && !ok {
		c.delete(c.head.key)
	}
	c.delete(key)
	node := &cacheNode[K, V]{
		key:       key,
		value:     value,
		createdAt: time.Now(),
	}
	c.insert(node)
}

func (c *LRUCache[K, V]) delete(key K) {
	node, ok := c.cacheMap[key]
	if !ok {
		return
	}
	delete(c.cacheMap, key)
	if c.head == nil {
		return
	}
	if c.head.key == key {
		c.head = c.head.next
		if c.head != nil {
			c.head.prev = nil
		}
		if c.tail != nil && c.tail.key == c.head.key {
			c.tail = nil
		}
		return
	}
	if c.tail == nil {
		return
	}
	if c.tail.key == key {
		c.tail = c.tail.prev
		c.tail.next = nil
		if c.head.key == c.tail.key {
			c.tail = nil
		}
		return
	}
	node.prev.next = node.next
	node.next.prev = node.prev
	node.next = nil
	node.prev = nil
}

func (c *LRUCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delete(key)
}

func (c *LRUCache[K, V]) insert(node *cacheNode[K, V]) {
	node.next = nil
	node.prev = nil
	c.cacheMap[node.key] = node
	if c.head == nil {
		c.head = node
	} else if c.tail == nil {
		c.tail = node
		c.tail.prev = c.head
		c.head.next = c.tail
	} else {
		c.tail.next = node
		node.prev = c.tail
		c.tail = node
	}
}
