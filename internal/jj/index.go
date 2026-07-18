package jj

import (
	"bytes"
	"container/heap"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	dipb "github.com/rselbach/jj-git-prompt/internal/jj/pb/default_indexpb"
)

// commitIndexFormatVersion is the segment file format this reader
// understands (jj 0.42).
const commitIndexFormatVersion = 6

const overflowFlag = 0x8000_0000

// Index is a read-only composite view of jj's default commit index: a chain
// of segment files, child first.
type Index struct {
	segments []*indexSegment // child first, root parent last
}

// indexSegment is one loaded commit index segment file.
//
// File layout after the header (see readonly.rs for the authoritative
// spec): graph entries, commit id lookup table, change id table, change
// position table, parent overflow table, change overflow table.
type indexSegment struct {
	numParentCommits  uint32
	numLocalCommits   uint32
	numLocalChangeIDs uint32
	commitIDLen       int
	changeIDLen       int

	graph          []byte
	commitLookup   []byte
	changeIDTable  []byte
	changePosTable []byte
	parentOverflow []byte
	changeOverflow []byte
}

// loadIndexAtOperation loads the index linked to the given operation, or
// walks ancestor operations to the nearest one that has an index link.
// jj-lib would build the missing index instead; a prompt can tolerate a
// slightly stale one. Returns nil if no linked index is found (the caller
// degrades gracefully).
func loadIndexAtOperation(repoPath string, op *Operation) (*Index, error) {
	indexDir := filepath.Join(repoPath, "index")
	if kind, err := os.ReadFile(filepath.Join(indexDir, "type")); err != nil || string(kind) != "default" {
		return nil, nil
	}

	visited := map[OperationID]bool{}
	queue := []*Operation{op}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.ID] {
			continue
		}
		visited[cur.ID] = true

		linkData, err := os.ReadFile(filepath.Join(indexDir, "op_links", cur.ID.Hex()))
		if err == nil {
			return loadIndexFromLink(indexDir, linkData)
		}
		for _, parentID := range cur.Parents {
			parent, err := readOperation(repoPath, parentID)
			if err != nil {
				continue
			}
			queue = append(queue, parent)
		}
	}
	return nil, nil
}

func loadIndexFromLink(indexDir string, linkData []byte) (*Index, error) {
	var link dipb.SegmentControl
	if err := proto.Unmarshal(linkData, &link); err != nil {
		return nil, fmt.Errorf("decode index op link: %w", err)
	}
	name := hex.EncodeToString(link.CommitSegmentId)
	ix := &Index{}
	dir := filepath.Join(indexDir, "segments")
	for name != "" {
		seg, parentName, err := loadIndexSegment(dir, name)
		if err != nil {
			return nil, err
		}
		ix.segments = append(ix.segments, seg)
		name = parentName
	}
	// Fill in cumulative parent commit counts (root parent has none).
	total := uint32(0)
	for i := len(ix.segments) - 1; i >= 0; i-- {
		ix.segments[i].numParentCommits = total
		total += ix.segments[i].numLocalCommits
	}
	return ix, nil
}

func loadIndexSegment(dir, name string) (seg *indexSegment, parentName string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, "", fmt.Errorf("read index segment: %w", err)
	}
	invalid := func(msg string) error {
		return fmt.Errorf("index segment %s: %s", name, msg)
	}
	next := func(n int) ([]byte, bool) {
		if len(data) < n {
			return nil, false
		}
		out := data[:n]
		data = data[n:]
		return out, true
	}
	nextU32 := func() (uint32, bool) {
		b, ok := next(4)
		if !ok {
			return 0, false
		}
		return binary.LittleEndian.Uint32(b), true
	}

	version, ok := nextU32()
	if !ok {
		return nil, "", invalid("truncated header")
	}
	if version != commitIndexFormatVersion {
		return nil, "", fmt.Errorf("index segment %s: format version %d, want %d (run any jj command to reindex)",
			name, version, commitIndexFormatVersion)
	}
	parentNameLen, ok := nextU32()
	if ok {
		var nameBytes []byte
		nameBytes, ok = next(int(parentNameLen))
		parentName = string(nameBytes)
	}
	numLocalCommits, ok1 := nextU32()
	numLocalChangeIDs, ok2 := nextU32()
	numParentOverflow, ok3 := nextU32()
	numChangeOverflow, ok4 := nextU32()
	if !ok || !ok1 || !ok2 || !ok3 || !ok4 {
		return nil, "", invalid("truncated header")
	}

	seg = &indexSegment{
		numLocalCommits:   numLocalCommits,
		numLocalChangeIDs: numLocalChangeIDs,
		commitIDLen:       commitIDLength,
		changeIDLen:       changeIDLength,
	}
	var okAll bool
	sections := []struct {
		dst  *[]byte
		size int
	}{
		{&seg.graph, int(numLocalCommits) * seg.graphEntrySize()},
		{&seg.commitLookup, int(numLocalCommits) * 4},
		{&seg.changeIDTable, int(numLocalChangeIDs) * seg.changeIDLen},
		{&seg.changePosTable, int(numLocalChangeIDs) * 4},
		{&seg.parentOverflow, int(numParentOverflow) * 4},
		{&seg.changeOverflow, int(numChangeOverflow) * 4},
	}
	for _, s := range sections {
		*s.dst, okAll = next(s.size)
		if !okAll {
			return nil, "", invalid("truncated data")
		}
	}
	if len(data) != 0 {
		return nil, "", invalid("unexpected trailing data")
	}
	return seg, parentName, nil
}

// graph entry accessors

func (s *indexSegment) graphEntrySize() int { return 16 + s.commitIDLen }

func (s *indexSegment) entry(localPos uint32) []byte {
	size := s.graphEntrySize()
	return s.graph[int(localPos)*size:][:size]
}

func (s *indexSegment) commitIDBytes(localPos uint32) []byte {
	return s.entry(localPos)[16 : 16+s.commitIDLen]
}

func (s *indexSegment) changeIDLookupPos(localPos uint32) uint32 {
	return binary.LittleEndian.Uint32(s.entry(localPos)[12:16])
}

// parentPositions returns global positions of the entry's parents.
func (s *indexSegment) parentPositions(localPos uint32) []uint32 {
	entry := s.entry(localPos)
	p1 := binary.LittleEndian.Uint32(entry[4:8])
	p2 := binary.LittleEndian.Uint32(entry[8:12])
	if p1&overflowFlag == 0 {
		if p2&overflowFlag == 0 {
			return []uint32{p1, p2}
		}
		return []uint32{p1}
	}
	// Overflow table: p1 is the bit-negated overflow position, p2 the
	// bit-negated parent count. "No parent" entries (0xffffffff) negate to
	// zero, so they take this path and yield an empty slice.
	overflowPos := ^p1
	numParents := ^p2
	out := make([]uint32, 0, numParents)
	for i := uint32(0); i < numParents; i++ {
		off := int(overflowPos+i) * 4
		out = append(out, binary.LittleEndian.Uint32(s.parentOverflow[off:]))
	}
	return out
}

func (s *indexSegment) commitLookupLocalPos(lookupPos uint32) uint32 {
	return binary.LittleEndian.Uint32(s.commitLookup[int(lookupPos)*4:])
}

func (s *indexSegment) changeIDAt(lookupPos uint32) []byte {
	return s.changeIDTable[int(lookupPos)*s.changeIDLen:][:s.changeIDLen]
}

func (s *indexSegment) changeLookupPos(lookupPos uint32) uint32 {
	return binary.LittleEndian.Uint32(s.changePosTable[int(lookupPos)*4:])
}

// binarySearch returns (pos, found): the position of the first element >=
// key per cmp, mirroring binary_search_pos_by.
func binarySearch(size uint32, cmp func(pos uint32) int) (uint32, bool) {
	lo, hi := uint32(0), size
	for lo < hi {
		mid := (lo + hi) / 2
		switch c := cmp(mid); {
		case c < 0:
			lo = mid + 1
		case c > 0:
			hi = mid
		default:
			return mid, true
		}
	}
	return lo, false
}

// composite operations

// CommitPos returns the global index position of a commit id.
func (ix *Index) CommitPos(id CommitID) (uint32, bool) {
	key := []byte(id)
	for _, seg := range ix.segments {
		pos, found := binarySearch(seg.numLocalCommits, func(pos uint32) int {
			return bytes.Compare(seg.commitIDBytes(seg.commitLookupLocalPos(pos)), key)
		})
		if found {
			return seg.numParentCommits + seg.commitLookupLocalPos(pos), true
		}
	}
	return 0, false
}

// segmentFor locates the segment holding a global position.
func (ix *Index) segmentFor(globalPos uint32) (*indexSegment, uint32) {
	for _, seg := range ix.segments {
		if globalPos >= seg.numParentCommits {
			return seg, globalPos - seg.numParentCommits
		}
	}
	panic(fmt.Sprintf("index position %d out of range", globalPos))
}

// CommitIDAt returns the commit id at a global position.
func (ix *Index) CommitIDAt(globalPos uint32) CommitID {
	seg, local := ix.segmentFor(globalPos)
	return CommitID(seg.commitIDBytes(local))
}

// ParentPositions returns the parents of the commit at a global position.
func (ix *Index) ParentPositions(globalPos uint32) []uint32 {
	seg, local := ix.segmentFor(globalPos)
	return seg.parentPositions(local)
}

// ShortestUniqueChangeIDPrefixLen mirrors
// shortest_unique_change_id_prefix_len: one digit more than the longest
// common hex prefix with the neighboring indexed change ids (hidden ones
// included).
func (ix *Index) ShortestUniqueChangeIDPrefixLen(id ChangeID) int {
	key := []byte(id)
	var prevID, nextID []byte
	for _, seg := range ix.segments {
		pos, found := binarySearch(seg.numLocalChangeIDs, func(pos uint32) int {
			return bytes.Compare(seg.changeIDAt(pos), key)
		})
		prevPos, nextPos := neighborPositions(pos, found, seg.numLocalChangeIDs)
		if prevPos >= 0 {
			prevID = maxBytes(prevID, seg.changeIDAt(uint32(prevPos)))
		}
		if nextPos >= 0 {
			nextID = minBytes(nextID, seg.changeIDAt(uint32(nextPos)))
		}
	}
	length := 0
	for _, neighbor := range [][]byte{prevID, nextID} {
		if neighbor != nil {
			length = max(length, commonHexLen(key, neighbor)+1)
		}
	}
	return length
}

// neighborPositions mirrors PositionLookupResult::neighbors; -1 means no
// neighbor on that side.
func neighborPositions(pos uint32, found bool, size uint32) (prev, next int64) {
	prev, next = int64(pos)-1, int64(pos)
	if found {
		next++
	}
	if next >= int64(size) {
		next = -1
	}
	return prev, next
}

func maxBytes(acc, b []byte) []byte {
	if acc == nil || bytes.Compare(b, acc) > 0 {
		return b
	}
	return acc
}

func minBytes(acc, b []byte) []byte {
	if acc == nil || bytes.Compare(b, acc) < 0 {
		return b
	}
	return acc
}

// ChangeIDPositions returns the global positions of all indexed commits
// (visible or hidden) with the given change id.
func (ix *Index) ChangeIDPositions(id ChangeID) []uint32 {
	key := []byte(id)
	var out []uint32
	for _, seg := range ix.segments {
		lookupPos, found := binarySearch(seg.numLocalChangeIDs, func(pos uint32) int {
			return bytes.Compare(seg.changeIDAt(pos), key)
		})
		if !found {
			continue
		}
		changePos := seg.changeLookupPos(lookupPos)
		if changePos&overflowFlag == 0 {
			out = append(out, seg.numParentCommits+changePos)
			continue
		}
		// Overflow: consecutive local positions until one belongs to a
		// different change id.
		for overflowPos := ^changePos; int(overflowPos)*4 < len(seg.changeOverflow); overflowPos++ {
			localPos := binary.LittleEndian.Uint32(seg.changeOverflow[int(overflowPos)*4:])
			if seg.changeIDLookupPos(localPos) != lookupPos {
				break
			}
			out = append(out, seg.numParentCommits+localPos)
		}
	}
	return out
}

// posHeap is a max-heap of global commit positions.
type posHeap []uint32

func (h posHeap) Len() int           { return len(h) }
func (h posHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h posHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *posHeap) Push(x any)        { *h = append(*h, x.(uint32)) }
func (h *posHeap) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// Reachable reports, for each target position, whether it is an ancestor of
// (or equal to) any of the head commits. This is the visibility test behind
// jj's divergent-change detection.
func (ix *Index) Reachable(headIDs []CommitID, targets []uint32) []bool {
	result := make([]bool, len(targets))
	if len(targets) == 0 {
		return result
	}
	minTarget := targets[0]
	for _, t := range targets[1:] {
		minTarget = min(minTarget, t)
	}

	visited := map[uint32]bool{}
	h := &posHeap{}
	for _, id := range headIDs {
		if pos, ok := ix.CommitPos(id); ok && pos >= minTarget && !visited[pos] {
			visited[pos] = true
			heap.Push(h, pos)
		}
	}
	for h.Len() > 0 {
		pos := heap.Pop(h).(uint32)
		for _, parent := range ix.ParentPositions(pos) {
			if parent < minTarget || visited[parent] {
				continue
			}
			visited[parent] = true
			heap.Push(h, parent)
		}
	}
	for i, t := range targets {
		result[i] = visited[t]
	}
	return result
}
