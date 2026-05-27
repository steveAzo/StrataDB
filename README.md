# StrataDB

A log-structured merge-tree (LSM-tree) key-value storage engine built from scratch in Go, implementing the core concepts from *Designing Data-Intensive Applications* Chapter 3.

Supports durable reads and writes, crash recovery, multi-level compaction, bloom filter-accelerated lookups, and an HTTP API.

---

## Architecture

### Write Path

```
Put("key", "value")
        │
        ▼
┌───────────────┐
│     WAL       │  ← append-only log, fsynced to disk first
│  (wal/)       │    crash? replay this to rebuild the memtable
└───────┬───────┘
        │
        ▼
┌───────────────┐
│   Memtable    │  ← sorted in-memory map (fast O(1) writes)
│  (memtable/)  │    holds writes until size threshold is hit
└───────┬───────┘
        │ ShouldFlush() == true
        ▼
┌───────────────┐
│  L0 SSTable   │  ← immutable sorted file on disk
│  (sstable/)   │    entries written in key order
└───────┬───────┘
        │ len(L0) >= maxL0Files
        ▼
┌───────────────┐
│  L1 SSTable   │  ← merged, deduplicated, tombstones dropped
│ (compaction/) │    one file representing the oldest stable data
└───────────────┘
```

### Read Path

```
Get("key")
        │
        ▼
┌───────────────┐
│   Memtable    │  ← check newest data first
└───────┬───────┘
        │ not found
        ▼
┌───────────────┐     ┌──────────────┐
│  Bloom Filter │─ No ─▶  skip file  │  ← definitely not here
│  (bloom/)     │     └──────────────┘
└───────┬───────┘
        │ maybe
        ▼
┌───────────────┐
│ L0 SSTables   │  ← newest → oldest (L0 files can overlap)
│ indexed seek  │    read footer → binary search index → seek to record
└───────┬───────┘
        │ not found
        ▼
┌───────────────┐
│ L1 SSTables   │  ← compacted, non-overlapping key ranges
│ indexed seek  │
└───────┬───────┘
        │ not found
        ▼
   ErrNotFound
```

### On-Disk SSTable Format

Every SSTable file has three sections:

```
┌──────────────────────────────────────────────────────────────┐
│  Data section                                                │
│  ┌──────────┬────────────┬──────────┬────────────┬────────┐ │
│  │ key_len  │  key bytes │ val_len  │ val bytes  │deleted │ │
│  │ uint32   │ variable   │ uint32   │ variable   │ uint8  │ │
│  └──────────┴────────────┴──────────┴────────────┴────────┘ │
│  (one record per entry, in sorted key order)                 │
├──────────────────────────────────────────────────────────────┤
│  Index section                                               │
│  ┌──────────┬────────────┬──────────────────────┐           │
│  │ key_len  │  key bytes │  offset (uint64)     │           │
│  └──────────┴────────────┴──────────────────────┘           │
│  (one entry per record — keys only, no values)               │
├──────────────────────────────────────────────────────────────┤
│  Footer (last 12 bytes)                                      │
│  ┌────────────────────────┬──────────────────┐               │
│  │  indexOffset (uint64)  │ numEntries(uint32)│              │
│  └────────────────────────┴──────────────────┘               │
└──────────────────────────────────────────────────────────────┘
```

The index enables `Get` to skip the data section entirely: read footer → seek to index → binary search → seek to exact record offset. No full-file scan.

The WAL uses the same length-prefix framing as the data section — one binary layout throughout the entire system.

### Bloom Filter (Double Hashing)

```
Add("apple"):
  h1, h2 = FNV-1a("apple")
  for i in 0..2:
    bit = (h1 + i*h2) % numBits
    set that bit

MayContain("banana"):
  if any of the 3 bits is 0 → DEFINITELY NOT HERE → skip SSTable
  all bits 1               → MAYBE HERE          → read the file
```

---

## Project Layout

```
stratadb/
├── memtable/       sorted in-memory write buffer
├── sstable/        immutable sorted files (indexed writer + reader)
├── wal/            write-ahead log for crash recovery
├── compaction/     merge SSTables, deduplicate, drop tombstones
├── bloom/          probabilistic filter to skip SSTables on reads
├── db/             top-level DB: orchestrates all of the above
├── server/         HTTP API: PUT/GET/DELETE /keys/{key}
└── main.go         entry point: starts the HTTP server
```

---

## HTTP API

```bash
# store a value
curl -X PUT http://localhost:6380/keys/name -d "strata"

# retrieve a value
curl http://localhost:6380/keys/name
# → strata

# delete a key
curl -X DELETE http://localhost:6380/keys/name
```

Start the server:

```bash
go run . -dir ./data -addr :6380
```

---

## Benchmarks

Measured on Windows 11, Intel i3-1125G4 @ 2.00GHz, Go 1.25.

```
BenchmarkPut-8                183992      27895 ns/op    →  ~35,800 writes/sec
BenchmarkGet_Hit-8               801    7600336 ns/op    →    ~130 reads/sec
BenchmarkGet_Miss-8         66207846         83 ns/op    →  ~12M misses/sec
BenchmarkPut_Concurrent-8    180295      31213 ns/op    →  ~32,000 writes/sec
```

**Write throughput (35K writes/sec):** Each write fsyncs the WAL before returning — the durability guarantee costs ~28µs per write. All writes are sequential I/O (WAL append + SSTable flush), which is why LSM-trees outperform B-trees on write-heavy workloads. B-trees do random I/O to update pages in place; LSM-trees never modify existing files.

**Read hit (130 reads/sec / ~7ms per read):** This benchmark uses small 5-byte values, so the index section and data section are nearly the same size (~24KB each for 1000 keys). The real cost is repeated file open/seek/close system calls on Windows — the OS cache warms up, but syscall overhead (~50–200µs each × 4 seeks per lookup) dominates. With larger values (1KB+), the indexed seek provides a 40× reduction in bytes read vs a full file scan, and the improvement is dramatic.

**Read miss (83ns / 12M misses/sec):** The bloom filter intercepts every SSTable in nanoseconds — three bit-checks in an in-memory byte array. Zero disk I/O. This is 90,000× faster than a read hit, which is the point: in workloads where many lookups come back empty (cache misses, existence checks), bloom filters are the difference between a usable and unusable database.

**Concurrent writes (32K writes/sec):** Nearly identical to single-threaded writes despite 8 competing goroutines. Writes serialize through `sync.RWMutex` — the lock isn't held long enough for contention to matter. The fsync cost dominates.

---

## Key Design Decisions

| Decision | Why |
|---|---|
| Map + sort-on-flush for memtable | O(1) writes; sorted order only needed at flush time |
| Immutable SSTables | No locking on reads; safe to merge concurrently |
| Length-prefix framing | Self-describing records; no delimiter scanning |
| WAL fsynced on every append | Durability: no write is lost on crash |
| Tombstones instead of deletes | Can't modify immutable files; compaction cleans them up |
| Index section in every SSTable | Skip data scan entirely: binary search index → seek to record |
| Bloom filters in memory | Avoids disk reads for keys that definitely don't exist |
| Last-write-wins in compaction | Simplest correct merge strategy for a single-writer DB |
| Scan dir on Open | Survives restarts: recover SSTable levels and WAL from disk |

---

## What's Not Implemented (Known Trade-offs)

| Missing | Production solution |
|---|---|
| Index caching | LevelDB's `TableCache` keeps parsed indexes in an LRU cache — eliminates repeated index reads per lookup |
| Sparse index | Store one index entry per N records (LevelDB: one per 4KB block) — reduces index size for large SSTables |
| Block compression | LevelDB/RocksDB compress each 4KB block with Snappy/LZ4 |
| MVCC | PostgreSQL keeps multiple row versions so readers don't block writers |
| Manifest file | LevelDB tracks SSTable membership in a `MANIFEST` log; we infer it from filenames |
| Crash-safe compaction | If the process dies mid-compaction, orphan files are left behind |

---


## Similar Production Systems

| System | Uses |
|---|---|
| LevelDB / RocksDB | LSM-tree, same L0→L1 compaction model, block indexes |
| Apache Cassandra | LSM-tree per column family, bloom filters per SSTable |
| PostgreSQL | B-tree storage, identical WAL concept, `shared_buffers` for page caching |
| InfluxDB | LSM-tree for time-series data |
