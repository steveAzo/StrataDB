package sstable

import (
	"os"
	"path/filepath"
	"testing"

	"stratadb/memtable"
)

func TestWriteAndFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sst")

	entries := []memtable.Entry{
		{Key: "apple", Value: "red", Deleted: false},
		{Key: "banana", Value: "yellow", Deleted: false},
		{Key: "cherry", Value: "", Deleted: true},
	}

	if err := Write(path, entries); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("SSTable file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("SSTable file is empty")
	}
}

func TestWriteEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sst")

	if err := Write(path, []memtable.Entry{}); err != nil {
		t.Fatalf("Write of empty entries failed: %v", err)
	}
}
