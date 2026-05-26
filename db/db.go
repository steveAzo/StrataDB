package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"stratadb/bloom"
	"stratadb/compaction"
	"stratadb/memtable"
	"stratadb/sstable"
	"stratadb/wal"
)

// ErrNotFound is returned by Get when the key does not exist.
var ErrNotFound = errors.New("key not found")

// DB is a log-structured key-value store backed by a memtable, a WAL,
// and a two-level SSTable hierarchy (L0 and L1).
//
// Write path:  WAL → memtable → flush to L0 → compact L0 into L1
// Read path:   memtable → L0 (newest→oldest) → L1
type DB struct {
	mu      sync.RWMutex
	dir     string
	mem     *memtable.Memtable
	w       *wal.WAL
	levels  [][]string               // levels[0]=L0 paths, levels[1]=L1 paths
	filters map[string]*bloom.Filter // SSTable path → bloom filter
	maxL0   int
	maxMem  int // stored so we can recreate the memtable after flush
	seq     int // counter for unique SSTable filenames
}

// Open opens or creates a DB in dir, recovering state from any existing
// SSTable files and replaying the WAL to restore in-flight writes.
func Open(dir string, maxMemBytes, maxL0Files int) (*DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db := &DB{
		dir:     dir,
		mem:     memtable.New(maxMemBytes),
		levels:  [][]string{{}, {}},
		filters: make(map[string]*bloom.Filter),
		maxL0:   maxL0Files,
		maxMem:  maxMemBytes,
	}

	// Recover SSTable files from the previous run.
	if err := db.loadExisting(); err != nil {
		return nil, err
	}

	// Replay WAL to recover any writes that were in the memtable when the
	// process last stopped. Must happen after loadExisting so the sequence
	// counter is already set past all on-disk file numbers.
	walPath := filepath.Join(dir, "wal")
	if _, err := os.Stat(walPath); err == nil {
		entries, err := wal.Replay(walPath)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Deleted {
				db.mem.Delete(e.Key)
			} else {
				db.mem.Put(e.Key, e.Value)
			}
		}
	}

	w, err := wal.Open(walPath)
	if err != nil {
		return nil, err
	}
	db.w = w
	return db, nil
}

// loadExisting scans dir for SSTable files from a previous run, populates
// db.levels and db.filters, and sets db.seq to the highest seen file number.
//
// Filenames look like "L0-00001.sst" or "L1-00003.sst".
// os.ReadDir returns entries alphabetically, which is also ascending numeric
// order for zero-padded names — so levels[0] is already oldest→newest.
func (db *DB) loadExisting() error {
	dirEntries, err := os.ReadDir(db.dir)
	if err != nil {
		return err
	}

	for _, entry := range dirEntries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sst") {
			continue
		}

		var levelIdx int 
		if strings.HasPrefix(name, "L0-") {
			levelIdx = 0
		} else if strings.HasPrefix(name, "L1-") {
			levelIdx = 1
		} else {
			continue
		}

		seqStr := strings.TrimSuffix(name[3:], ".sst")
		seq, err := strconv.Atoi(seqStr)
		if err != nil {
			continue
		}
		if seq > db.seq {
			db.seq = seq
		}
		//
		// Add path to the right level and rebuild its bloom filter:
		path := filepath.Join(db.dir, name)
		db.levels[levelIdx] = append(db.levels[levelIdx], path)
		entries, err := sstable.ReadAll(path)
		if err != nil {
			return err
		}
		f := bloom.New(uint64(len(entries)+1) * 10)
		for _, e := range entries {
			f.Add(e.Key)
		}
		db.filters[path] = f

		_ = name
	}
	return nil
}

// Close flushes the memtable to disk and closes the WAL.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.flush(); err != nil {
		return err
	}
	return db.w.Close()
}

// Put stores a key-value pair durably.
func (db *DB) Put(key, value string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	e := memtable.Entry{Key: key, Value: value}
	if err := db.w.Append(e); err != nil {
		return err
	}
	db.mem.Put(key, value)
	if db.mem.ShouldFlush() {
		return db.flush()
	}
	return nil
}

// Delete marks a key as deleted.
func (db *DB) Delete(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	e := memtable.Entry{Key: key, Deleted: true}
	if err := db.w.Append(e); err != nil {
		return err
	}
	db.mem.Delete(key)
	if db.mem.ShouldFlush() {
		return db.flush()
	}
	return nil
}

// Get retrieves the value for key. Returns ErrNotFound if absent or deleted.
func (db *DB) Get(key string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if e, ok := db.mem.Get(key); ok {
		if e.Deleted {
			return "", ErrNotFound
		}
		return e.Value, nil
	}

	for i := len(db.levels[0]) - 1; i >= 0; i-- {
		path := db.levels[0][i]
		if f, ok := db.filters[path]; ok && !f.MayContain(key) {
			continue
		}
		if e, err := sstable.Get(path, key); err == nil {
			if e.Deleted {
				return "", ErrNotFound
			}
			return e.Value, nil
		}
	}

	for _, path := range db.levels[1] {
		if f, ok := db.filters[path]; ok && !f.MayContain(key) {
			continue
		}
		if e, err := sstable.Get(path, key); err == nil {
			if e.Deleted {
				return "", ErrNotFound
			}
			return e.Value, nil
		}
	}

	return "", ErrNotFound
}

// flush writes the current memtable to a new L0 SSTable and resets the memtable.
// Triggers L0→L1 compaction if L0 has reached maxL0 files.
// Caller must hold db.mu.
func (db *DB) flush() error {
	if db.mem.Size() == 0 {
		return nil
	}
	path := db.nextPath("L0")
	entries := db.mem.Entries()
	if err := sstable.Write(path, entries); err != nil {
		return err
	}
	f := bloom.New(uint64(len(entries)+1) * 10)
	for _, e := range entries {
		f.Add(e.Key)
	}
	db.filters[path] = f

	db.levels[0] = append(db.levels[0], path)
	db.mem = memtable.New(db.maxMem)

	if len(db.levels[0]) >= db.maxL0 {
		return db.compactL0()
	}
	return nil
}

// compactL0 merges all L0 SSTables into a single L1 SSTable and clears L0.
// Caller must hold db.mu.
func (db *DB) compactL0() error {
	if len(db.levels[0]) == 0 {
		return nil
	}
	outPath := db.nextPath("L1")
	if err := compaction.Merge(db.levels[0], outPath); err != nil {
		return err
	}
	for _, p := range db.levels[0] {
		os.Remove(p)
		delete(db.filters, p)
	}
	db.levels[0] = nil

	// rebuild bloom filter for the new merged L1 file
	entries, err := sstable.ReadAll(outPath)
	if err != nil {
		return err
	}
	f := bloom.New(uint64(len(entries)+1) * 10)
	for _, e := range entries {
		f.Add(e.Key)
	}
	db.filters[outPath] = f
	db.levels[1] = append(db.levels[1], outPath)
	return nil
}

// nextPath returns a unique SSTable path for the given level label.
func (db *DB) nextPath(level string) string {
	db.seq++
	return filepath.Join(db.dir, fmt.Sprintf("%s-%05d.sst", level, db.seq))
}
