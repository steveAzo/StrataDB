package sstable

import (
	"encoding/binary"
	"io"
	"os"

	"stratadb/memtable"
)

// File layout:
//
//	[data section  ] one record per entry, length-prefix framing
//	[index section ] one (key, offset) pair per entry — keys only, no values
//	[footer        ] 8-byte indexOffset + 4-byte numEntries = 12 bytes total
//
// The index lets Get seek directly to a record without reading the whole file.
// indexOffset is the byte position where the index section starts.

const footerSize = 12 // uint64 + uint32

// Write flushes a sorted slice of memtable entries to a new SSTable at path.
// It writes the data section, then the index, then the footer, then fsyncs.
func Write(path string, entries []memtable.Entry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	offsets := make([]uint64, len(entries))
	for i, e := range entries {
		off, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		offsets[i] = uint64(off)
		if err := writeRecord(f, e); err != nil {
			return err
		}
	}
	
	indexOffset, _ := f.Seek(0, io.SeekCurrent)
	for i, e := range entries {
		if err := writeIndexEntry(f, e.Key, offsets[i]); err != nil {
			return err
		}
	}
	
	binary.Write(f, binary.BigEndian, uint64(indexOffset))
	binary.Write(f, binary.BigEndian, uint32(len(entries)))

	_ = io.SeekCurrent
	_ = entries
	return f.Sync()
}

// writeRecord encodes one Entry into length-prefix binary format.
func writeRecord(w io.Writer, e memtable.Entry) error {
	keyBytes := []byte(e.Key)
	valBytes := []byte(e.Value)
	binary.Write(w, binary.BigEndian, uint32(len(keyBytes)))
	w.Write(keyBytes)
	binary.Write(w, binary.BigEndian, uint32(len(valBytes)))
	w.Write(valBytes)
	return binary.Write(w, binary.BigEndian, boolToByte(e.Deleted))
}

// writeIndexEntry writes one index record: key length, key bytes, byte offset.
func writeIndexEntry(w io.Writer, key string, offset uint64) error {
	keyBytes := []byte(key)
	binary.Write(w, binary.BigEndian, uint32(len(keyBytes)))
	w.Write(keyBytes)
	return binary.Write(w, binary.BigEndian, offset)
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
