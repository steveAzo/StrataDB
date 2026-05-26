package sstable

import (
	"errors"
	"path/filepath"
	"testing"

	"stratadb/memtable"
)

var testEntries = []memtable.Entry{
	{Key: "apple", Value: "red", Deleted: false},
	{Key: "banana", Value: "yellow", Deleted: false},
	{Key: "cherry", Value: "", Deleted: true},
	{Key: "date", Value: "brown", Deleted: false},
}

func writeTempSSTable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sst")
	if err := Write(path, testEntries); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	return path
}

func TestReadAllRoundTrip(t *testing.T) {
	path := writeTempSSTable(t)

	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if len(entries) != len(testEntries) {
		t.Fatalf("expected %d entries, got %d", len(testEntries), len(entries))
	}
	for i, got := range entries {
		want := testEntries[i]
		if got.Key != want.Key || got.Value != want.Value || got.Deleted != want.Deleted {
			t.Errorf("entry %d: got %+v, want %+v", i, got, want)
		}
	}
}

func TestGetExisting(t *testing.T) {
	path := writeTempSSTable(t)

	e, err := Get(path, "banana")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if e.Value != "yellow" {
		t.Fatalf("expected 'yellow', got %q", e.Value)
	}
}

func TestGetTombstone(t *testing.T) {
	path := writeTempSSTable(t)

	e, err := Get(path, "cherry")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !e.Deleted {
		t.Fatal("expected cherry to be a tombstone")
	}
}

func TestGetMissing(t *testing.T) {
	path := writeTempSSTable(t)

	_, err := Get(path, "mango")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
