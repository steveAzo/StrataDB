package compaction

import (
	"errors"
	"path/filepath"
	"testing"

	"stratadb/memtable"
	"stratadb/sstable"
)

func writeSSTable(t *testing.T, dir, name string, entries []memtable.Entry) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := sstable.Write(path, entries); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	return path
}

func TestMergeDeduplicates(t *testing.T) {
	dir := t.TempDir()

	// older SSTable — has stale value for "color"
	a := writeSSTable(t, dir, "a.sst", []memtable.Entry{
		{Key: "color", Value: "blue"},
		{Key: "fruit", Value: "apple"},
	})
	// newer SSTable — overwrites "color"
	b := writeSSTable(t, dir, "b.sst", []memtable.Entry{
		{Key: "color", Value: "green"},
		{Key: "size", Value: "large"},
	})

	out := filepath.Join(dir, "merged.sst")
	if err := Merge([]string{a, b}, out); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	entries, err := sstable.ReadAll(out)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	index := make(map[string]string)
	for _, e := range entries {
		index[e.Key] = e.Value
	}

	if index["color"] != "green" {
		t.Errorf("expected color=green (newest wins), got %q", index["color"])
	}
	if index["fruit"] != "apple" {
		t.Errorf("expected fruit=apple, got %q", index["fruit"])
	}
	if index["size"] != "large" {
		t.Errorf("expected size=large, got %q", index["size"])
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries after merge, got %d", len(entries))
	}
}

func TestMergeDropsTombstones(t *testing.T) {
	dir := t.TempDir()

	a := writeSSTable(t, dir, "a.sst", []memtable.Entry{
		{Key: "ghost", Value: "boo"},
	})
	b := writeSSTable(t, dir, "b.sst", []memtable.Entry{
		{Key: "ghost", Value: "", Deleted: true},
	})

	out := filepath.Join(dir, "merged.sst")
	if err := Merge([]string{a, b}, out); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	entries, err := sstable.ReadAll(out)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (tombstone dropped), got %d", len(entries))
	}

	// The key should no longer be findable
	_, err = sstable.Get(out, "ghost")
	if !errors.Is(err, sstable.ErrNotFound) {
		t.Errorf("expected ErrNotFound for deleted key, got %v", err)
	}
}

func TestMergeOutputIsSorted(t *testing.T) {
	dir := t.TempDir()

	a := writeSSTable(t, dir, "a.sst", []memtable.Entry{
		{Key: "zebra", Value: "z"},
		{Key: "apple", Value: "a"},
	})

	out := filepath.Join(dir, "merged.sst")
	if err := Merge([]string{a}, out); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	entries, err := sstable.ReadAll(out)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "apple" || entries[1].Key != "zebra" {
		t.Errorf("output not sorted: got %q, %q", entries[0].Key, entries[1].Key)
	}
}
