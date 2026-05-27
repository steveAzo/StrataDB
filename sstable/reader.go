package sstable

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sort"

	"stratadb/memtable"
)

// ErrNotFound is returned by Get when the key does not exist in this SSTable.
var ErrNotFound = errors.New("key not found")

// footer holds the two values packed into the last 12 bytes of every SSTable.
type footer struct {
	indexOffset uint64 // byte position where the index section starts
	numEntries  uint32 // number of index (and data) entries
}

// indexEntry is one record from the index section: a key and the byte offset
// of its corresponding data record in the data section.
type indexEntry struct {
	Key    string
	Offset uint64
}

// readFooter reads the 12-byte footer from the end of f.
func readFooter(f *os.File) (footer, error) {
	if _, err := f.Seek(-footerSize, io.SeekEnd); err != nil {
		return footer{}, err
	}
	var ft footer
	binary.Read(f, binary.BigEndian, &ft.indexOffset)
	binary.Read(f, binary.BigEndian, &ft.numEntries)
	return ft, nil
}

// readIndex seeks to the index section and reads all (key, offset) pairs.
func readIndex(f *os.File, ft footer) ([]indexEntry, error) {
	if _, err := f.Seek(int64(ft.indexOffset), io.SeekStart); err != nil {
		return nil, err
	}
	entries := make([]indexEntry, ft.numEntries)
	for i := range entries {
		var keyLen uint32
		binary.Read(f, binary.BigEndian, &keyLen)
		keyBytes := make([]byte, keyLen)
		io.ReadFull(f, keyBytes)
		var offset uint64
		binary.Read(f, binary.BigEndian, &offset)
		entries[i] = indexEntry{Key: string(keyBytes), Offset: offset}
	}
	return entries, nil
}

// ReadAll reads every data record from the SSTable at path.
// It reads only up to indexOffset so it never tries to decode index bytes as records.
func ReadAll(path string) ([]memtable.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ft, err := readFooter(f)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var entries []memtable.Entry
	for {
		pos, _ := f.Seek(0, io.SeekCurrent)
		if uint64(pos) >= ft.indexOffset {
			break
		}
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

// Get looks up key in the SSTable using the index — no full file scan.
// It reads the footer, loads the index, binary searches for the key,
// then seeks directly to the matching data record.
func Get(path, key string) (memtable.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return memtable.Entry{}, err
	}
	defer f.Close()

	// TODO: read footer, read index, binary search for key, seek to record offset, return readRecord(f)
	//
	ft, err := readFooter(f)
	if err != nil {
		return memtable.Entry{}, err
	}
	index, err := readIndex(f, ft)
	if err != nil {
		return memtable.Entry{}, err
	}
	i := sort.Search(len(index), func(i int) bool { return index[i].Key >= key })
	if i >= len(index) || index[i].Key != key {
		return memtable.Entry{}, ErrNotFound
	}
	if _, err := f.Seek(int64(index[i].Offset), io.SeekStart); err != nil {
		return memtable.Entry{}, err
	}
	return readRecord(f)

	// _ = sort.Search
	// return memtable.Entry{}, ErrNotFound
}

// readRecord decodes one Entry from r using length-prefix framing.
// Returns io.EOF when there are no more records.
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
