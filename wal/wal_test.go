package wal

import (
	"path/filepath"
	"testing"

	"stratadb/memtable"
)

func TestAppendAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wal")

	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	entries := []memtable.Entry{
		{Key: "dog", Value: "woof", Deleted: false},
		{Key: "cat", Value: "meow", Deleted: false},
		{Key: "bird", Value: "", Deleted: true},
	}
	for _, e := range entries {
		if err := w.Append(e); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
	w.Close()

	got, err := Replay(path)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(got))
	}
	for i, e := range got {
		want := entries[i]
		if e.Key != want.Key || e.Value != want.Value || e.Deleted != want.Deleted {
			t.Errorf("entry %d: got %+v, want %+v", i, e, want)
		}
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delete.wal")

	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := w.Delete(path); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := Open(path); err != nil {
		// file was deleted — opening it should create a new one, not error
		t.Fatalf("expected Open to recreate WAL, got: %v", err)
	}
}
