package sstable

import (
	"encoding/binary"
	"io"
	"os"

	"stratadb/memtable"
)

// Record layout on disk (one per entry, written sequentially):
//
//	[key_len   uint32 - 4 bytes]
//	[key       bytes  - key_len bytes]
//	[value_len uint32 - 4 bytes]
//	[value     bytes  - value_len bytes]
//	[deleted   uint8  - 1 byte]  0 = live, 1 = tombstone
//
// Entries are written in sorted key order (the memtable guarantees this
// via Entries()). The file is never modified after creation.
//
// Why length-prefix framing? Same reason as LogStream: no delimiters to
// scan for, no escaping needed. Each record is fully self-describing.

// Write flushes a sorted slice of memtable entries to a new SSTable file
// at the given path. The file is fsynced before returning so the data
// survives a crash. Returns an error if any write fails.
func Write(path string, entries []memtable.Entry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range entries {
		if err := writeRecord(f, e); err != nil {
			return err
		}
	}

	// fsync so the OS flushes its write buffer to disk.
	// Without this, a crash after Write returns could leave an empty file.
	return f.Sync()
}

// writeRecord encodes one Entry into the length-prefix binary format and
// writes it to w. Uses binary.Write with big-endian byte order.
func writeRecord(w io.Writer, e memtable.Entry) error {
	keyBytes := []byte(e.Key)
	valBytes := []byte(e.Value)

	binary.Write(w, binary.BigEndian, uint32(len(keyBytes)))
	w.Write(keyBytes)
	binary.Write(w, binary.BigEndian, uint32(len(valBytes)))
	w.Write(valBytes)
	return binary.Write(w, binary.BigEndian, boolToByte(e.Deleted))

}

// boolToByte converts a bool to 0 or 1 for on-disk storage.
func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
