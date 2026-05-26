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

// ReadAll reads every record from the SSTable file at path and returns
// them as a slice. Records are already in sorted key order (the writer
// guarantees this), so the returned slice is sorted too.
func ReadAll(path string) ([]memtable.Entry, error) {
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

// readRecord decodes one Entry from r using the same length-prefix format
// the writer used. Returns io.EOF when there are no more records.
func readRecord(r io.Reader) (memtable.Entry, error) {
	// TODO: mirror writeRecord exactly, but read instead of write
	// Read key length, then key bytes, then value length, then value bytes, then deleted flag.
	//
	// var keyLen uint32
	// if err := binary.Read(r, binary.BigEndian, &keyLen); err != nil {
	// 	return memtable.Entry{}, err  // io.EOF here means clean end-of-file
	// }
	// keyBytes := make([]byte, keyLen)
	// io.ReadFull(r, keyBytes)
	//
	// var valLen uint32
	// binary.Read(r, binary.BigEndian, &valLen)
	// valBytes := make([]byte, valLen)
	// io.ReadFull(r, valBytes)
	//
	// var deleted uint8
	// binary.Read(r, binary.BigEndian, &deleted)
	//
	// return memtable.Entry{
	// 	Key:     string(keyBytes),
	// 	Value:   string(valBytes),
	// 	Deleted: deleted == 1,
	// }, nil
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
		Key: string(keyBytes),
		Value: string(valBytes),
		Deleted: deleted == 1,
	}, nil 


}

// Get searches the SSTable for the given key.
// Returns ErrNotFound if the key is absent or was tombstoned.
func Get(path, key string) (memtable.Entry, error) {
	entries, err := ReadAll(path)
	if err != nil {
		return memtable.Entry{}, err
	}

	// TODO: binary search entries for key using sort.Search, return ErrNotFound if missing
	// sort.Search returns the smallest index i where entries[i].Key >= key.
	// If that index is in bounds and entries[i].Key == key, you found it.
	//
	// i := sort.Search(len(entries), func(i int) bool {
	// 	return entries[i].Key >= key
	// })
	// if i < len(entries) && entries[i].Key == key {
	// 	return entries[i], nil
	// }
	// return memtable.Entry{}, ErrNotFound
	i := sort.Search(len(entries), func(i int) bool {
		return entries[i].Key >= key
	})
	if i < len(entries) && entries[i].Key == key {
		return entries[i], nil 
	}

	_ = sort.Search
	return memtable.Entry{}, ErrNotFound
}
