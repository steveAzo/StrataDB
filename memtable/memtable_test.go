package memtable

import (
	"testing"
)

func TestPutAndGet(t *testing.T) {
	m := New(1024)

	m.Put("banana", "yellow")
	m.Put("apple", "red")
	m.Put("cherry", "red")

	entry, ok := m.Get("apple")
	if !ok {
		t.Fatal("expected to find 'apple'")
	}
	if entry.Value != "red" {
		t.Fatalf("expected 'red', got %q", entry.Value)
	}
	if entry.Deleted {
		t.Fatal("expected apple to not be deleted")
	}
}

func TestOverwrite(t *testing.T) {
	m := New(1024)

	m.Put("color", "blue")
	m.Put("color", "green") // overwrite

	entry, ok := m.Get("color")
	if !ok {
		t.Fatal("expected to find 'color'")
	}
	if entry.Value != "green" {
		t.Fatalf("expected 'green', got %q", entry.Value)
	}
}

func TestDelete(t *testing.T) {
	m := New(1024)

	m.Put("ghost", "boo")
	m.Delete("ghost")

	entry, ok := m.Get("ghost")
	if !ok {
		t.Fatal("expected to find tombstone for 'ghost'")
	}
	if !entry.Deleted {
		t.Fatal("expected ghost to be marked deleted")
	}
}

func TestGetMissing(t *testing.T) {
	m := New(1024)

	_, ok := m.Get("nothing")
	if ok {
		t.Fatal("expected missing key to return ok=false")
	}
}

func TestEntriesSorted(t *testing.T) {
	m := New(1024)

	m.Put("zebra", "z")
	m.Put("apple", "a")
	m.Put("mango", "m")

	entries := m.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	expected := []string{"apple", "mango", "zebra"}
	for i, e := range entries {
		if e.Key != expected[i] {
			t.Fatalf("position %d: expected %q, got %q", i, expected[i], e.Key)
		}
	}
}

func TestShouldFlush(t *testing.T) {
	// maxSize of 10 bytes — tiny so we can trigger it easily
	m := New(10)

	if m.ShouldFlush() {
		t.Fatal("should not flush on empty memtable")
	}

	m.Put("key", "value") // 3 + 5 = 8 bytes, still under 10

	// Adding one more byte should push it over
	m.Put("k2", "vvv") // 2 + 3 = 5 more bytes, total = 13 >= 10

	if !m.ShouldFlush() {
		t.Fatal("expected ShouldFlush to return true")
	}
}

func TestSizeTracking(t *testing.T) {
	m := New(1024)

	m.Put("hello", "world") // 5 + 5 = 10
	if m.Size() != 10 {
		t.Fatalf("expected size 10, got %d", m.Size())
	}

	m.Put("hello", "go") // overwrite: subtract 10, add 5+2=7
	if m.Size() != 7 {
		t.Fatalf("expected size 7 after overwrite, got %d", m.Size())
	}
}
