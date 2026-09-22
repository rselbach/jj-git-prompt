package jj

import (
	"container/heap"
	"io"
	"slices"

	"github.com/go-git/go-git/v5/plumbing"
)

// This file approximates jj's is_empty check for merge commits: the commit
// is empty when its tree equals the auto-merge of its parent trees
// (merge_commit_trees in rewrite.rs). We verify that top-down at the tree
// entry level. Merges that would need file-content merging, criss-cross
// ancestry, or conflicted parents return "unknown" and the caller treats
// the commit as non-empty.

// isMergeCommitEmpty reports whether a 2+-parent commit's tree matches the
// merged parent trees. Only the 2-parent, single-ancestor case is handled.
func (r *Repo) isMergeCommitEmpty(commit *Commit) bool {
	if r.index == nil || len(commit.Parents) != 2 {
		return false
	}
	base, ok := r.commonAncestorTree(commit.Parents[0], commit.Parents[1])
	if !ok {
		return false
	}
	p1, err1 := r.ReadCommit(commit.Parents[0])
	p2, err2 := r.ReadCommit(commit.Parents[1])
	if err1 != nil || err2 != nil || p1.HasConflict() || p2.HasConflict() {
		return false
	}
	t1, t0, t2 := p1.RootTree[0], base, p2.RootTree[0]

	// A conflicted WC tree is empty when its terms are exactly the
	// unresolved merge of the parents (a fresh `jj new a b` with conflicts).
	if commit.HasConflict() {
		return slices.Equal(commit.RootTree, []string{t1, t0, t2})
	}
	return r.verifyMergedTree(t1, t0, t2, commit.RootTree[0])
}

// commonAncestorTree returns the root tree of the single greatest common
// ancestor of two commits, or ok=false for criss-cross or conflicted
// ancestors.
func (r *Repo) commonAncestorTree(a, b CommitID) (string, bool) {
	posA, okA := r.index.CommitPos(a)
	posB, okB := r.index.CommitPos(b)
	if !okA || !okB {
		return "", false
	}
	gcas := r.index.commonAncestors(posA, posB)
	if len(gcas) != 1 {
		return "", false
	}
	ancestor, err := r.ReadCommit(r.index.CommitIDAt(gcas[0]))
	if err != nil || ancestor.HasConflict() {
		return "", false
	}
	return ancestor.RootTree[0], true
}

// verifyMergedTree reports whether c is the trivial (entry-level) 3-way
// merge of t1/t0/t2. Entries that would need file-content merging fail the
// verification.
func (r *Repo) verifyMergedTree(t1, t0, t2, c string) bool {
	// Trivial resolutions at the whole-tree level.
	switch {
	case t1 == t2:
		return c == t1
	case t0 == t1:
		return c == t2
	case t0 == t2:
		return c == t1
	}

	e1, err1 := r.backend.readTree(t1)
	e0, err0 := r.backend.readTree(t0)
	e2, err2 := r.backend.readTree(t2)
	ec, errc := r.backend.readTree(c)
	if err1 != nil || err0 != nil || err2 != nil || errc != nil {
		return false
	}

	names := map[string]bool{}
	for _, entries := range []map[string]treeEntry{e1, e0, e2, ec} {
		for name := range entries {
			names[name] = true
		}
	}
	for name := range names {
		v1, v0, v2, vc := e1[name], e0[name], e2[name], ec[name]
		var want treeEntry
		switch {
		case v1 == v2:
			want = v1
		case v0 == v1:
			want = v2
		case v0 == v2:
			want = v1
		case v1.isTree() && v0.isTree() && v2.isTree():
			if !vc.isTree() || !r.verifyMergedTree(v1.id, v0.id, v2.id, vc.id) {
				return false
			}
			continue
		default:
			// Would require file-content merging.
			return false
		}
		if vc != want {
			return false
		}
	}
	return true
}

const treeModeDir = "40000"

// treeEntry is a git tree entry; the zero value means absent.
type treeEntry struct {
	mode string
	id   string // raw object id bytes
}

func (e treeEntry) isTree() bool { return e.mode == treeModeDir }

// readTree parses a raw git tree object into its entries by name.
func (b *gitBackend) readTree(id string) (map[string]treeEntry, error) {
	hash := plumbing.NewHash(CommitID(id).Hex())
	obj, err := b.objects.EncodedObject(plumbing.TreeObject, hash)
	if err != nil {
		return nil, err
	}
	rd, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer rd.Close() //nolint:errcheck // read-only reader
	raw, err := io.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	entries := map[string]treeEntry{}
	for len(raw) > 0 {
		sp := slices.Index(raw, byte(' '))
		nul := slices.Index(raw, byte(0))
		if sp < 0 || nul < sp || len(raw) < nul+1+commitIDLength {
			break
		}
		mode := string(raw[:sp])
		name := string(raw[sp+1 : nul])
		oid := string(raw[nul+1 : nul+1+commitIDLength])
		entries[name] = treeEntry{mode: mode, id: oid}
		raw = raw[nul+1+commitIDLength:]
	}
	return entries, nil
}

// commonAncestors returns the maximal common ancestors of two index
// positions (git's paint-down-to-common).
func (ix *Index) commonAncestors(a, b uint32) []uint32 {
	const (
		flagA     = 1
		flagB     = 2
		flagBoth  = flagA | flagB
		flagStale = 4
	)
	flags := map[uint32]uint8{a: flagA, b: flagB}
	if a == b {
		return []uint32{a}
	}
	h := &posHeap{a, b}
	heap.Init(h)
	var results []uint32

	nonStaleLeft := func() bool {
		for _, pos := range *h {
			if flags[pos]&flagStale == 0 {
				return true
			}
		}
		return false
	}

	for h.Len() > 0 && nonStaleLeft() {
		pos := heap.Pop(h).(uint32)
		f := flags[pos]
		if f&flagBoth == flagBoth && f&flagStale == 0 {
			results = append(results, pos)
			f |= flagStale
			flags[pos] = f
		}
		// Parents always sort before children in the index, so their flags
		// are final by the time they pop; stale spreads through f.
		for _, parent := range ix.ParentPositions(pos) {
			old, seen := flags[parent]
			flags[parent] = old | f
			if !seen {
				heap.Push(h, parent)
			}
		}
	}
	return results
}
