package db

import (
	"fmt"
	"testing"
)

// BenchmarkPut measures write throughput: WAL append + memtable write + occasional flush.
// Run with: go test ./db/... -bench=BenchmarkPut -benchtime=5s
func BenchmarkPut(b *testing.B) {
	d, err := Open(b.TempDir(), 4*1024*1024, 4)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	b.ResetTimer() // don't count setup time
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%08d", i)
		if err := d.Put(key, "value"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGet_Hit measures read throughput when the key exists.
// This exercises the full read path: memtable miss → L0 SSTable → bloom filter → binary search.
// Run with: go test ./db/... -bench=BenchmarkGet_Hit -benchtime=5s
func BenchmarkGet_Hit(b *testing.B) {
	d, err := Open(b.TempDir(), 4*1024, 4) // small memtable to force SSTable flushes
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	const n = 1000
	for i := 0; i < n; i++ {
		d.Put(fmt.Sprintf("key-%08d", i), "value")
	}
	target := fmt.Sprintf("key-%08d", n/2) // pick a key in the middle
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Get(target); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGet_Miss measures read throughput when the key does NOT exist.
// This is the worst-case read path without bloom filters: every SSTable must be checked.
// With bloom filters, each file is skipped in nanoseconds (a few bit-checks in memory).
// Compare this number to BenchmarkGet_Hit to see the bloom filter overhead on misses.
// Run with: go test ./db/... -bench=BenchmarkGet_Miss -benchtime=5s
func BenchmarkGet_Miss(b *testing.B) {
	d, err := Open(b.TempDir(), 4*1024, 4)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	// pre-populate so there are SSTables on disk to check (and skip)
	for i := 0; i < 1000; i++ {
		d.Put(fmt.Sprintf("key-%08d", i), "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Get("zzz-this-key-does-not-exist") // bloom filter should skip all SSTables
	}
}

// BenchmarkPut_Concurrent measures write throughput under concurrent load.
// Uses b.RunParallel — each goroutine calls Put with a unique key.
// In StrataDB, writes serialize through the mutex (one at a time).
// This shows the contention cost of that design.
// Run with: go test ./db/... -bench=BenchmarkPut_Concurrent -cpu=1,2,4,8
func BenchmarkPut_Concurrent(b *testing.B) {
	d, err := Open(b.TempDir(), 4*1024*1024, 4)
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()

	var counter int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := fmt.Sprintf("key-%d", counter)
			counter++
			d.Put(id, "value")
		}
	})
}
