# StrataDB

A log-structured merge-tree (LSM-tree) key-value storage engine built from scratch in Go, implementing the core concepts from *Designing Data-Intensive Applications* Chapter 3.

---

## Architecture

### Write Path

```
Put("key", "value")
        │
        ▼
┌───────────────┐
│     WAL       │  ← append-only log, fsynced to disk first
│  (wal/)       │    crash? replay this to rebuild memtable
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
│ binary search │
└───────┬───────┘
        │ not found
        ▼
┌───────────────┐
│ L1 SSTables   │  ← compacted, non-overlapping key ranges
│ binary search │
└───────┬───────┘
        │ not found
        ▼
   ErrNotFound
```

### On-Disk Record Format

Used by both SSTable and WAL — a single binary layout throughout.

```
┌──────────────┬─────────────────┬──────────────────┬──────────────────┬──────────┐
│  key_len     │   key bytes     │    value_len     │   value bytes    │ deleted  │
│  (uint32)    │   (key_len B)   │    (uint32)      │   (value_len B)  │ (uint8)  │
│   4 bytes    │   variable      │    4 bytes       │   variable       │  1 byte  │
└──────────────┴─────────────────┴──────────────────┴──────────────────┴──────────┘
```

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
├── sstable/        immutable sorted files (writer + reader)
├── wal/            write-ahead log for crash recovery
├── compaction/     merge SSTables, deduplicate, drop tombstones
├── bloom/          probabilistic filter to skip SSTables on reads
└── db/             top-level DB: orchestrates all of the above
```

---

## Key Design Decisions

| Decision | Why |
|---|---|
| Map + sort-on-flush for memtable | O(1) writes; sorted order only needed at flush time |
| Immutable SSTables | No locking on reads; safe to merge concurrently |
| Length-prefix framing | Self-describing records; no delimiter scanning |
| WAL fsynced on every append | Durability guarantee: no write is lost on crash |
| Tombstones instead of deletes | Can't modify immutable files; compaction cleans them up |
| Bloom filters in memory | Avoids disk reads for keys that definitely don't exist |
| Last-write-wins in compaction | Simplest correct merge strategy for a single-writer DB |

---

## Concepts from DDIA Chapter 3

- **SSTables** — sorted, immutable on-disk files
- **LSM-Tree** — the write-optimized structure formed by memtable + SSTable levels
- **Compaction** — merging files to reclaim space and enforce newest-wins
- **WAL** — same crash-recovery principle used by PostgreSQL, MySQL, and RocksDB
- **Bloom filters** — probabilistic structure to reduce read amplification
- **Read/write amplification** — the core trade-off: LSM-trees optimize writes at the cost of reads

---

## Similar Production Systems

| System | Uses |
|---|---|
| LevelDB / RocksDB | LSM-tree, same L0→L1 compaction model |
| Apache Cassandra | LSM-tree per column family |
| PostgreSQL | B-tree storage, but identical WAL concept |
| InfluxDB | LSM-tree for time-series data |
