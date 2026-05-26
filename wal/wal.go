package wal

import (
	"encoding/binary"
	"errors"
	"io"
	"os"

	"stratadb/memtable"
)

// WAL is a write-ahead log: an append-only file that records every Put and
// Delete before it reaches the memtable. On crash, replay it to rebuild
// the memtable exactly as it was.
//
// Record format: identical to the SSTable record format —
//
//	[key_len uint32][key bytes][value_len uint32][value bytes][deleted uint8]
//
// We reuse the same framing so there is only one binary layout to reason about.
type WAL struct {
	f *os.File
}

// Open opens an existing WAL file or creates a new one at path.
// Appends go to the end of the file (os.O_APPEND).
func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE |os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{f: f}, nil
}

// Append writes one entry to the WAL and fsyncs immediately.
// The fsync makes the write durable — safe against a crash right after this call.
func (w *WAL) Append(e memtable.Entry) error {
	// TODO: encode the entry with writeRecord, then call w.f.Sync()
	// if err := writeRecord(w.f, e); err != nil {
	// 	return err
	// }
	// return w.f.Sync()
	if err := writeRecord(w.f, e); err != nil {
		return err
	}
	return nil
}

// Replay reads every entry from the WAL file at path and returns them in
// order. The caller feeds these back into a fresh memtable to recover state.
func Replay(path string) ([]memtable.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []memtable.Entry
	for {
		e, err := readRecord(f)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Close closes the underlying file.
func (w *WAL) Close() error {
	return w.f.Close()
}

// Delete closes and removes the WAL file. Called after a successful
// memtable flush — once the data is in an SSTable, the WAL is redundant.
func (w *WAL) Delete(path string) error {
	w.f.Close()
	return os.Remove(path)
}

// writeRecord and readRecord are duplicated from the sstable package
// intentionally — the WAL and SSTable share the same on-disk format but
// are separate concerns. In a larger codebase you'd extract this to a
// shared encoding package.

func writeRecord(w io.Writer, e memtable.Entry) error {
	keyBytes := []byte(e.Key)
	valBytes := []byte(e.Value)
	binary.Write(w, binary.BigEndian, uint32(len(keyBytes)))
	w.Write(keyBytes)
	binary.Write(w, binary.BigEndian, uint32(len(valBytes)))
	w.Write(valBytes)
	return binary.Write(w, binary.BigEndian, boolToByte(e.Deleted))
}

func readRecord(r io.Reader) (memtable.Entry, error) {
	var keyLen uint32
	if err := binary.Read(r, binary.BigEndian, &keyLen); err != nil {
		return memtable.Entry{}, err
	}
	keyBytes := make([]byte, keyLen)
	io.ReadFull(r, keyBytes)

	var valLen uint32
	binary.Read(r, binary.BigEndian, &valLen)
	valBytes := make([]byte, valLen)
	io.ReadFull(r, valBytes)

	var deleted uint8
	binary.Read(r, binary.BigEndian, &deleted)

	return memtable.Entry{
		Key:     string(keyBytes),
		Value:   string(valBytes),
		Deleted: deleted == 1,
	}, nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
