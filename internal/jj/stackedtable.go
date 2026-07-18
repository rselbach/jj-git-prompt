package jj

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// stackedTable is a read-only view of one segment of jj's stacked table (a
// sorted fixed-size-key → variable-size-value file with an optional parent
// segment forming a chain).
//
// Segment file format (all integers little-endian):
//
//	u32: parent file name length (0 = no parent)
//	<parent file name bytes>
//	u32: number of local entries
//	entries: [key keySize bytes][u32 value offset] * n, sorted by key
//	values:  concatenated value bytes
type stackedTable struct {
	parent  *stackedTable
	keySize int
	index   []byte // entry records
	values  []byte
}

const stackedEntryOffsetSize = 4

// loadStackedTables loads the head table chains for a table store directory.
// The heads directory contains one empty file per head table name. Multiple
// heads occur only after unmerged concurrent writes; jj would merge them on
// the next write, we simply search all of them.
func loadStackedTables(dir string, keySize int) ([]*stackedTable, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "heads"))
	if err != nil {
		return nil, fmt.Errorf("read table heads: %w", err)
	}
	var tables []*stackedTable
	for _, e := range entries {
		t, err := loadStackedTable(dir, e.Name(), keySize)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

func loadStackedTable(dir, name string, keySize int) (*stackedTable, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("read table segment: %w", err)
	}
	invalid := func(msg string) error {
		return fmt.Errorf("table segment %s: %s", name, msg)
	}
	if len(data) < 4 {
		return nil, invalid("truncated header")
	}
	parentNameLen := int(binary.LittleEndian.Uint32(data))
	data = data[4:]
	var parent *stackedTable
	if parentNameLen > 0 {
		if len(data) < parentNameLen {
			return nil, invalid("truncated parent name")
		}
		parent, err = loadStackedTable(dir, string(data[:parentNameLen]), keySize)
		if err != nil {
			return nil, err
		}
		data = data[parentNameLen:]
	}
	if len(data) < 4 {
		return nil, invalid("truncated entry count")
	}
	numEntries := int(binary.LittleEndian.Uint32(data))
	data = data[4:]
	indexSize := numEntries * (keySize + stackedEntryOffsetSize)
	if len(data) < indexSize {
		return nil, invalid("truncated index")
	}
	return &stackedTable{
		parent:  parent,
		keySize: keySize,
		index:   data[:indexSize],
		values:  data[indexSize:],
	}, nil
}

// get looks up a key in this segment and its parents.
func (t *stackedTable) get(key []byte) ([]byte, bool) {
	for ; t != nil; t = t.parent {
		if v, ok := t.localGet(key); ok {
			return v, true
		}
	}
	return nil, false
}

func (t *stackedTable) localGet(key []byte) ([]byte, bool) {
	entrySize := t.keySize + stackedEntryOffsetSize
	n := len(t.index) / entrySize
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		entry := t.index[mid*entrySize:]
		switch bytes.Compare(key, entry[:t.keySize]) {
		case -1:
			hi = mid
		case 1:
			lo = mid + 1
		default:
			return t.valueByPos(mid, n), true
		}
	}
	return nil, false
}

func (t *stackedTable) valueByPos(pos, numEntries int) []byte {
	entrySize := t.keySize + stackedEntryOffsetSize
	start := t.valueOffset(pos, entrySize)
	end := len(t.values)
	if pos+1 < numEntries {
		end = t.valueOffset(pos+1, entrySize)
	}
	return t.values[start:end]
}

func (t *stackedTable) valueOffset(pos, entrySize int) int {
	off := pos*entrySize + t.keySize
	return int(binary.LittleEndian.Uint32(t.index[off:]))
}
