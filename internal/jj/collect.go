package jj

import (
	"slices"
	"strings"
)

// Bookmark is a bookmark name with its ancestor distance from the working
// copy (0 = directly on the WC commit).
type Bookmark struct {
	Name     string
	Distance int
}

// Info is the jj repository status collected for the prompt, mirroring
// JjInfo in the Rust implementation.
type Info struct {
	// ChangeID is the shortened change id in reverse hex form.
	ChangeID string
	// ChangeIDPrefixLen is the shortest unique prefix length of the change
	// id, capped to len(ChangeID).
	ChangeIDPrefixLen int
	// Bookmarks on the WC commit or its ancestors, closest first.
	Bookmarks []Bookmark
	// EmptyDesc reports an empty commit description.
	EmptyDesc bool
	// EmptyCommit reports that the commit tree matches its parent tree.
	EmptyCommit bool
	// Conflict reports unresolved conflicts in the WC commit.
	Conflict bool
	// Divergent reports multiple visible commits with the same change id.
	Divergent bool
	// HasRemote reports whether the closest bookmark exists on any remote.
	HasRemote bool
	// IsSynced reports whether the closest bookmark agrees with a remote
	// counterpart (vacuously true with no bookmarks or no remotes).
	IsSynced bool
}

// Collect gathers prompt info for the workspace at root, mirroring
// collect() in the Rust implementation's jj.rs.
func Collect(root string, idLength, ancestorDepth int) (*Info, error) {
	repo, err := Load(root)
	if err != nil {
		return nil, err
	}

	wcID, err := repo.WCCommitID()
	if err != nil {
		return nil, err
	}
	commit, err := repo.ReadCommit(wcID)
	if err != nil {
		return nil, err
	}

	changeIDFull := EncodeReverseHex([]byte(commit.ChangeID))
	changeID := changeIDFull[:min(idLength, len(changeIDFull))]

	info := &Info{
		ChangeID:          changeID,
		ChangeIDPrefixLen: changeIDPrefixLen(repo, wcID, commit.ChangeID, idLength, len(changeID)),
		EmptyDesc:         strings.TrimSpace(commit.Description) == "",
		EmptyCommit:       isCommitEmpty(repo, commit),
		Conflict:          commit.HasConflict(),
		Divergent:         isDivergent(repo, commit.ChangeID),
	}

	for _, name := range repo.View.LocalBookmarksForCommit(wcID) {
		info.Bookmarks = append(info.Bookmarks, Bookmark{Name: name})
	}
	sortBookmarks(info.Bookmarks)
	if ancestorDepth > 0 {
		ancestors, err := ancestorBookmarks(repo, wcID, ancestorDepth)
		if err != nil {
			return nil, err
		}
		info.Bookmarks = append(info.Bookmarks, ancestors...)
	}

	info.HasRemote, info.IsSynced = remoteSyncState(repo.View, info.Bookmarks)
	return info, nil
}

// changeIDPrefixLen uses the index only when it is fresh enough to contain
// the WC commit; otherwise falls back to the configured id length.
func changeIDPrefixLen(repo *Repo, wcID CommitID, id ChangeID, fallback, maxLen int) int {
	length := fallback
	if repo.index != nil {
		if _, ok := repo.index.CommitPos(wcID); ok {
			length = repo.index.ShortestUniqueChangeIDPrefixLen(id)
		}
	}
	return min(length, maxLen)
}

// isCommitEmpty mirrors Commit::is_empty: the commit tree matches the
// (auto-merged) parent tree. Merge cases that would need file-content
// merging report non-empty; see treemerge.go.
func isCommitEmpty(repo *Repo, commit *Commit) bool {
	if len(commit.Parents) != 1 {
		return repo.isMergeCommitEmpty(commit)
	}
	parent, err := repo.ReadCommit(commit.Parents[0])
	if err != nil {
		return false
	}
	return slices.Equal(commit.RootTree, parent.RootTree)
}

// isDivergent reports more than one visible commit with the given change
// id, mirroring resolve_change_id + visible_with_offsets.
func isDivergent(repo *Repo, id ChangeID) bool {
	if repo.index == nil {
		return false
	}
	positions := repo.index.ChangeIDPositions(id)
	if len(positions) < 2 {
		return false
	}
	visible := 0
	for _, reachable := range repo.index.Reachable(repo.View.HeadIDs, positions) {
		if reachable {
			visible++
			if visible > 1 {
				return true
			}
		}
	}
	return false
}

// immutableHeads mirrors find_immutable_heads: trunk bookmarks, tags, and
// untracked remote bookmarks stop the ancestor search.
func immutableHeads(view *View) map[CommitID]bool {
	immutable := map[CommitID]bool{}
	for remote, rv := range view.RemoteViews {
		if remote == "git" {
			continue
		}
		for name, ref := range rv.Bookmarks {
			isTrunk := (remote == "origin" || remote == "upstream") &&
				(name == "main" || name == "master" || name == "trunk")
			isUntracked := view.LocalBookmark(name).IsAbsent()
			if !isTrunk && !isUntracked {
				continue
			}
			if id, ok := ref.Target.AsNormal(); ok {
				immutable[id] = true
			}
		}
	}
	for _, target := range view.LocalTags {
		if id, ok := target.AsNormal(); ok {
			immutable[id] = true
		}
	}
	return immutable
}

// ancestorBookmarks BFS-searches ancestors of the WC commit for local
// bookmarks, mirroring find_ancestor_bookmarks.
func ancestorBookmarks(repo *Repo, wcID CommitID, maxDepth int) ([]Bookmark, error) {
	type queued struct {
		id    CommitID
		depth int
	}
	immutable := immutableHeads(repo.View)
	distances := map[string]int{}
	visited := map[CommitID]bool{}

	wcParents, err := repo.parentIDs(wcID)
	if err != nil {
		return nil, err
	}
	var queue []queued
	for _, p := range wcParents {
		queue = append(queue, queued{p, 1})
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth > maxDepth || visited[cur.id] {
			continue
		}
		visited[cur.id] = true

		for _, name := range repo.View.LocalBookmarksForCommit(cur.id) {
			if _, seen := distances[name]; !seen {
				distances[name] = cur.depth
			}
		}
		if immutable[cur.id] || cur.depth >= maxDepth {
			continue
		}
		parents, err := repo.parentIDs(cur.id)
		if err != nil {
			return nil, err
		}
		for _, p := range parents {
			queue = append(queue, queued{p, cur.depth + 1})
		}
	}

	bookmarks := make([]Bookmark, 0, len(distances))
	for name, distance := range distances {
		bookmarks = append(bookmarks, Bookmark{Name: name, Distance: distance})
	}
	sortBookmarks(bookmarks)
	return bookmarks, nil
}

// sortBookmarks orders by distance, then name for determinism (the Rust
// version leaves equal distances in hash order).
func sortBookmarks(bookmarks []Bookmark) {
	slices.SortFunc(bookmarks, func(a, b Bookmark) int {
		if a.Distance != b.Distance {
			return a.Distance - b.Distance
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// remoteSyncState mirrors the has_remote/is_synced computation for the
// closest bookmark.
func remoteSyncState(view *View, bookmarks []Bookmark) (hasRemote, isSynced bool) {
	if len(bookmarks) == 0 {
		return false, true
	}
	name := bookmarks[0].Name
	localTarget := view.LocalBookmark(name)
	for remote, rv := range view.RemoteViews {
		if remote == "git" {
			continue
		}
		ref, ok := rv.Bookmarks[name]
		if !ok {
			continue
		}
		hasRemote = true
		if ref.Target.Equal(localTarget) {
			isSynced = true
			break
		}
	}
	return hasRemote, isSynced || !hasRemote
}
