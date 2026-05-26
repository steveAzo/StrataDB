package db

import (
	"errors"
	"path/filepath"
	"testing"

	"stratadb/memtable"
	"stratadb/wal"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir(), 64, 3) // 64-byte memtable, compact after 3 L0 files
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestPutAndGet(t *testing.T) {
	d := openTestDB(t)

	if err := d.Put("name", "strata"); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	val, err := d.Get("name")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "strata" {
		t.Errorf("expected 'strata', got %q", val)
	}
}

func TestGetMissing(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Get("ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestOverwrite(t *testing.T) {
	d := openTestDB(t)

	d.Put("k", "v1")
	d.Put("k", "v2")

	val, err := d.Get("k")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "v2" {
		t.Errorf("expected 'v2' (newest wins), got %q", val)
	}
}

func TestDelete(t *testing.T) {
	d := openTestDB(t)

	d.Put("temp", "here")
	d.Delete("temp")

	_, err := d.Get("temp")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestFlushAndReadFromSSTable(t *testing.T) {
	// maxMemBytes=1 forces a flush on every Put
	d, err := Open(t.TempDir(), 1, 10)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	d.Put("persistent", "yes")

	val, err := d.Get("persistent")
	if err != nil {
		t.Fatalf("Get failed after flush: %v", err)
	}
	if val != "yes" {
		t.Errorf("expected 'yes', got %q", val)
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	// First session: write some data and close cleanly.
	d1, err := Open(dir, 64, 4)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	d1.Put("name", "strata")
	d1.Put("version", "1")
	d1.Close()

	// Second session: open the same dir — data must survive.
	d2, err := Open(dir, 64, 4)
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer d2.Close()

	val, err := d2.Get("name")
	if err != nil {
		t.Fatalf("Get('name') after restart: %v", err)
	}
	if val != "strata" {
		t.Errorf("expected 'strata', got %q", val)
	}

	val, err = d2.Get("version")
	if err != nil {
		t.Fatalf("Get('version') after restart: %v", err)
	}
	if val != "1" {
		t.Errorf("expected '1', got %q", val)
	}
}

func TestWALReplayAfterCrash(t *testing.T) {
	dir := t.TempDir()

	// Simulate in-flight writes at crash time by writing directly to the WAL.
	// We close the WAL cleanly so the file handle is released — we can't
	// actually kill a process in a unit test, but this exercises the same
	// code path: Open must replay the WAL when no SSTable exists yet.
	w, err := wal.Open(filepath.Join(dir, "wal"))
	if err != nil {
		t.Fatalf("WAL open failed: %v", err)
	}
	w.Append(memtable.Entry{Key: "crash", Value: "survived"})
	w.Close()

	// Open the DB — loadExisting finds no SSTables, WAL replay recovers the entry.
	d, err := Open(dir, 1<<20, 4)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	val, err := d.Get("crash")
	if err != nil {
		t.Fatalf("Get after WAL replay: %v", err)
	}
	if val != "survived" {
		t.Errorf("expected 'survived', got %q", val)
	}
}

func TestL0Compaction(t *testing.T) {
	// maxL0=2 so compaction triggers after 2 flushes
	d, err := Open(t.TempDir(), 1, 2)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	d.Put("a", "1")
	d.Put("b", "2")
	d.Put("a", "updated") // overwrites "a" — compaction must keep newest

	val, err := d.Get("a")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "updated" {
		t.Errorf("expected 'updated' after compaction, got %q", val)
	}
}
