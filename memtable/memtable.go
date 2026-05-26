package memtable

import (
	"sort"
	"sync"
)

// Entry is one record in the memtable.
// Deleted=true means this is a tombstone — the key has been deleted.
// We write tombstones instead of removing keys because the real data
// might already be in an SSTable on disk; only compaction can erase it.
type Entry struct {
	Key     string
	Value   string
	Deleted bool
}

// Memtable is the in-memory write buffer for StrataDB.
// Every Put and Delete lands here first, then gets flushed to an
// immutable SSTable on disk once the buffer is full.
//
// We use a map for O(1) reads/writes and sort only at flush time.
// Trade-off: a skip list (used by LevelDB) keeps entries always-sorted,
// but adds complexity we don't need yet.
type Memtable struct {
	mu      sync.RWMutex
	data    map[string]Entry
	size    int // running byte count: sum of len(key)+len(value) per entry
	maxSize int // flush threshold; when size >= maxSize, flush to SSTable
}

// New creates an empty Memtable with the given flush threshold (in bytes).
func New(maxSize int) *Memtable {
	return &Memtable{
		data:    make(map[string]Entry),
		maxSize: maxSize,
	}
}

// Put inserts or overwrites a key-value pair.
func (m *Memtable) Put(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if old, ok := m.data[key]; ok {
		m.size -= len(old.Key) + len(old.Value)
	}

	m.data[key] = Entry{Key: key, Value: value, Deleted: false}
	m.size += len(key) + len(value)
}

// Delete marks a key as deleted by writing a tombstone.
// Does NOT remove the key — compaction handles that later.
func (m *Memtable) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if old, ok := m.data[key]; ok {
		m.size -= len(old.Key) + len(old.Value)
	}
	m.data[key] = Entry{Key: key, Deleted: true}
	m.size += len(key)
}

// Get returns the Entry for a key and whether it was found.
// Caller must check Entry.Deleted — a tombstone means treat as missing.
func (m *Memtable) Get(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.data[key]
	return entry, ok
}

// ShouldFlush reports whether the memtable has hit its size threshold.
func (m *Memtable) ShouldFlush() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.size >= m.maxSize
}

// Entries returns all entries sorted by key — called by the flush path.
func (m *Memtable) Entries() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]Entry, 0, len(m.data))
	for _, e := range m.data {
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries
}

// Size returns the current approximate byte size of the memtable.
func (m *Memtable) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}
