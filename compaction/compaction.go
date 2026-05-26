package compaction

import (
	"sort"

	"stratadb/memtable"
	"stratadb/sstable"
)

// Merge reads all entries from the input SSTable paths (oldest first),
// deduplicates by key (last write wins), drops tombstones, sorts the result,
// and writes it to outputPath as a new SSTable.
//
// Input order matters: paths[0] is the oldest SSTable, paths[len-1] is newest.
// When the same key appears in multiple files, the version from the newest
// file (highest index) is the one that survives.
func Merge(paths []string, outputPath string) error {
	merged := make(map[string]memtable.Entry)

	for _, path := range paths {
		entries, err := sstable.ReadAll(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			merged[e.Key] = e
		}
	}

	result := collectAndSort(merged)

	return sstable.Write(outputPath, result)
}

// collectAndSort converts the merged map into a sorted slice, dropping tombstones.
func collectAndSort(merged map[string]memtable.Entry) []memtable.Entry {
	entries := make([]memtable.Entry, 0, len(merged))

	for _, e := range merged {
		if !e.Deleted {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	_ = sort.Slice
	return entries
}
