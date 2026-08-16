package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLRU(t *testing.T) {
	cache := NewLRU[string, int](3)

	require.NotNil(t, cache, "NewLRU returned nil")
	require.Equal(t, 3, cache.capacity)
	require.NotNil(t, cache.cacheMap, "cacheMap is nil")
	require.Nil(t, cache.head, "head should be nil for a new cache")
	require.Nil(t, cache.tail, "tail should be nil for a new cache")
}

func TestLRUPutAndGet(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("two", 2)

	value, ok := cache.Get("one")
	require.True(t, ok, "expected key one to exist")
	assert.Equal(t, 1, value)

	value, ok = cache.Get("two")
	require.True(t, ok, "expected key two to exist")
	assert.Equal(t, 2, value)
}

func TestLRUGetMissingKey(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)

	value, ok := cache.Get("missing")
	require.False(t, ok, "expected missing key to return false")
	assert.Equal(t, 0, value)
}

func TestLRUPutUpdatesExistingValue(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("one", 100)

	value, ok := cache.Get("one")
	require.True(t, ok, "expected key one to exist")
	assert.Equal(t, 100, value)
	assert.Equal(t, 1, len(cache.cacheMap))
}

func TestLRUEviction(t *testing.T) {
	cache := NewLRU[string, int](2)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)

	_, ok := cache.Get("one")
	assert.False(t, ok, "expected the least recently used key one to be evicted")

	value, ok := cache.Get("two")
	require.True(t, ok, "expected key two to exist")
	assert.Equal(t, 2, value)

	value, ok = cache.Get("three")
	require.True(t, ok, "expected key three to exist")
	assert.Equal(t, 3, value)
}

func TestLRUGetChangesOrder(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)

	// Order before Get:
	// one -> two -> three
	//
	// Accessing one should make it most recently used:
	// two -> three -> one
	_, ok := cache.Get("one")
	require.True(t, ok, "expected key one to exist")

	cache.Put("four", 4)

	_, ok = cache.Get("two")
	assert.False(t, ok, "expected key two to be evicted")

	_, ok = cache.Get("three")
	assert.True(t, ok, "expected key three to remain")

	_, ok = cache.Get("one")
	assert.True(t, ok, "expected key one to remain because it was recently accessed")

	_, ok = cache.Get("four")
	assert.True(t, ok, "expected key four to remain")
}

func TestLRUOrderAfterMultipleGets(t *testing.T) {
	cache := NewLRU[string, int](4)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)
	cache.Put("four", 4)

	// Initial:
	// one -> two -> three -> four
	//
	// Get two:
	// one -> three -> four -> two
	_, _ = cache.Get("two")

	// Get one:
	// three -> four -> two -> one
	_, _ = cache.Get("one")

	// Adding five should evict three.
	cache.Put("five", 5)

	_, ok := cache.Get("three")
	assert.False(t, ok, "expected three to be evicted")

	for _, key := range []string{"one", "two", "four", "five"} {
		_, ok := cache.Get(key)
		assert.True(t, ok, "expected key %q to remain in cache", key)
	}
}

func TestLRUDeleteHead(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)

	cache.Delete("one")

	_, ok := cache.Get("one")
	assert.False(t, ok, "expected one to be deleted")

	require.NotNil(t, cache.head, "head should not be nil")
	assert.Equal(t, "two", cache.head.key)
	assert.Nil(t, cache.head.prev, "head.prev should be nil")
}

func TestLRUDeleteTail(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)

	cache.Delete("three")

	_, ok := cache.Get("three")
	assert.False(t, ok, "expected three to be deleted")

	require.NotNil(t, cache.tail, "tail should not be nil")
	assert.Equal(t, "two", cache.tail.key)
	assert.Nil(t, cache.tail.next, "tail.next should be nil")
}

func TestLRUDeleteMiddle(t *testing.T) {
	cache := NewLRU[string, int](3)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)

	cache.Delete("two")

	_, ok := cache.Get("two")
	assert.False(t, ok, "expected two to be deleted")

	require.NotNil(t, cache.head)
	require.NotNil(t, cache.tail)
	assert.Equal(t, "one", cache.head.key)
	assert.Equal(t, "three", cache.tail.key)
	assert.Same(t, cache.tail, cache.head.next, "head.next should point to tail")
	assert.Same(t, cache.head, cache.tail.prev, "tail.prev should point to head")
}

func TestLRUDeleteMissingKey(t *testing.T) {
	cache := NewLRU[string, int](2)

	cache.Put("one", 1)
	cache.Put("two", 2)

	cache.Delete("missing")

	assert.Equal(t, 2, len(cache.cacheMap))
}

func TestLRUCapacityOne(t *testing.T) {
	cache := NewLRU[string, int](1)

	cache.Put("one", 1)

	value, ok := cache.Get("one")
	require.True(t, ok)
	assert.Equal(t, 1, value)

	cache.Put("two", 2)

	_, ok = cache.Get("one")
	assert.False(t, ok, "expected one to be evicted")

	value, ok = cache.Get("two")
	require.True(t, ok)
	assert.Equal(t, 2, value)
}

func TestLRUSingleElementDelete(t *testing.T) {
	cache := NewLRU[string, int](1)

	cache.Put("one", 1)
	cache.Delete("one")

	assert.Empty(t, cache.cacheMap)
	assert.Nil(t, cache.head, "head should be nil")
	assert.Nil(t, cache.tail, "tail should be nil")
}

func TestLRUGenericTypes(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	cache := NewLRU[int, User](2)

	cache.Put(1, User{ID: 1, Name: "Alice"})
	cache.Put(2, User{ID: 2, Name: "Bob"})

	user, ok := cache.Get(1)
	require.True(t, ok, "expected user 1 to exist")

	expected := User{ID: 1, Name: "Alice"}
	assert.Equal(t, expected, user)
}

func TestLRUStringValues(t *testing.T) {
	cache := NewLRU[string, string](2)

	cache.Put("language", "Go")
	cache.Put("version", "1.26")

	value, ok := cache.Get("language")
	require.True(t, ok, "expected language to exist")
	assert.Equal(t, "Go", value)
}

func TestLRUInternalListConsistency(t *testing.T) {
	cache := NewLRU[string, int](4)

	cache.Put("one", 1)
	cache.Put("two", 2)
	cache.Put("three", 3)
	cache.Put("four", 4)

	require.NotNil(t, cache.head, "head should not be nil")
	require.NotNil(t, cache.tail, "tail should not be nil")

	// Walk from head to tail and verify prev/next links.
	var previous *cacheNode[string, int]
	count := 0

	for node := cache.head; node != nil; node = node.next {
		assert.Same(t, previous, node.prev, "invalid prev link for key %q", node.key)
		assert.Same(t, node, cache.cacheMap[node.key], "cacheMap points to wrong node for key %q", node.key)

		previous = node
		count++
	}

	assert.Same(t, cache.tail, previous, "last node should be tail")
	assert.Equal(t, count, len(cache.cacheMap),
		"linked list contains %d nodes, cacheMap contains %d", count, len(cache.cacheMap))
}

func TestLRUConcurrentAccess(t *testing.T) {
	cache := NewLRU[int, int](100)

	const goroutines = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()

			for i := 0; i < operations; i++ {
				key := id*operations + i

				cache.Put(key, key)

				_, _ = cache.Get(key)
			}
		}(g)
	}

	wg.Wait()

	assert.LessOrEqual(t, len(cache.cacheMap), cache.capacity,
		"cache size %d exceeds capacity %d", len(cache.cacheMap), cache.capacity)
}

func TestLRURepeatedPutDoesNotGrowCache(t *testing.T) {
	cache := NewLRU[string, int](3)

	for i := 0; i < 100; i++ {
		cache.Put("same", i)
	}

	assert.Equal(t, 1, len(cache.cacheMap))

	value, ok := cache.Get("same")
	require.True(t, ok, "expected same to exist")
	assert.Equal(t, 99, value)
}
