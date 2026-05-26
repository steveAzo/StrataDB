package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
	mu     sync.RWMutex
	dir    string               // directory holding all SSTable and WAL files
	mem    *memtable.Memtable
	w      *wal.WAL
	levels [][]string           // levels[0]=L0 paths, levels[1]=L1 paths
	maxL0  int                  // compact L0 into L1 when len(levels[0]) >= maxL0
	seq    int                  // counter for unique SSTable filenames
}

// Open opens or creates a DB in dir. Creates the directory if needed.
// maxMemBytes is the memtable flush threshold; maxL0Files triggers L0 compaction.
func Open(dir string, maxMemBytes, maxL0Files int) (*DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	w, err := wal.Open(filepath.Join(dir, "wal"))
	if err != nil {
		return nil, err
	}
	return &DB{
		dir:    dir,
		mem:    memtable.New(maxMemBytes),
		w:      w,
		levels: [][]string{{}, {}}, // L0 and L1 start empty
		maxL0:  maxL0Files,
	}, nil
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
		if e, err := sstable.Get(db.levels[0][i], key); err == nil {
			if e.Deleted {
				return "", ErrNotFound
			}
			return e.Value, nil
		}
	}

	for _, path := range db.levels[1] {
		if e, err := sstable.Get(path, key); err == nil {
			if e.Deleted {
				return  "", ErrNotFound
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
	if err := sstable.Write(path, db.mem.Entries()); err != nil {
		return err
	}
	db.levels[0] = append(db.levels[0], path)
	db.mem = memtable.New(db.mem.Size()) // reset; reuse same threshold

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
	// clean up the L0 files that were merged away
	for _, p := range db.levels[0] {
		os.Remove(p)
	}
	db.levels[0] = nil
	db.levels[1] = append(db.levels[1], outPath)
	return nil
}

// nextPath returns a unique SSTable path for the given level label.
func (db *DB) nextPath(level string) string {
	db.seq++
	return filepath.Join(db.dir, fmt.Sprintf("%s-%05d.sst", level, db.seq))
}
