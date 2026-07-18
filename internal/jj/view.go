package jj

import (
	pb "github.com/rselbach/jj-git-prompt/internal/jj/pb/simple_op_storepb"
)

// RemoteRef is a bookmark or tag on a remote.
type RemoteRef struct {
	Target RefTarget
	// Tracked reports whether the remote ref is tracked by a local ref.
	Tracked bool
}

// RemoteView is the bookmarks/tags of one remote.
type RemoteView struct {
	Bookmarks map[string]RemoteRef
	Tags      map[string]RemoteRef
}

// View is the repo view at one operation: heads, bookmarks, tags, and
// working-copy commits.
type View struct {
	HeadIDs        []CommitID
	WCCommitIDs    map[string]CommitID // by workspace name
	LocalBookmarks map[string]RefTarget
	LocalTags      map[string]RefTarget
	RemoteViews    map[string]RemoteView // by remote name
}

// LocalBookmark returns the local bookmark target, or an absent target if
// the bookmark doesn't exist.
func (v *View) LocalBookmark(name string) RefTarget {
	if t, ok := v.LocalBookmarks[name]; ok {
		return t
	}
	return absentRefTarget()
}

// LocalBookmarksForCommit returns names of local bookmarks whose added ids
// contain the commit.
func (v *View) LocalBookmarksForCommit(id CommitID) []string {
	var names []string
	for name, target := range v.LocalBookmarks {
		for _, added := range target.AddedIDs() {
			if added == id {
				names = append(names, name)
				break
			}
		}
	}
	return names
}

// viewFromProto mirrors view_from_proto in simple_op_store.rs, including the
// legacy per-bookmark remote lists (pre jj 0.34).
func viewFromProto(proto *pb.View) *View {
	v := &View{
		WCCommitIDs:    map[string]CommitID{},
		LocalBookmarks: map[string]RefTarget{},
		LocalTags:      map[string]RefTarget{},
		RemoteViews:    map[string]RemoteView{},
	}
	for _, id := range proto.HeadIds {
		v.HeadIDs = append(v.HeadIDs, CommitID(id))
	}
	if len(proto.WcCommitId) > 0 { //nolint:staticcheck // legacy single-workspace field, as jj-lib reads it
		v.WCCommitIDs["default"] = CommitID(proto.WcCommitId) //nolint:staticcheck // legacy field
	}
	for name, id := range proto.WcCommitIds {
		v.WCCommitIDs[name] = CommitID(id)
	}
	for _, bm := range proto.Bookmarks {
		local := refTargetFromProto(bm.LocalTarget)
		if !local.IsAbsent() {
			v.LocalBookmarks[bm.Name] = local
		}
		// Legacy embedded remote bookmarks (deprecated since jj 0.34).
		for _, rb := range bm.RemoteBookmarks { //nolint:staticcheck // legacy field read for pre-0.34 repos
			rv := v.remoteView(rb.RemoteName)
			tracked := rb.State != nil && *rb.State == pb.RemoteRefState_Tracked
			rv.Bookmarks[bm.Name] = RemoteRef{
				Target:  refTargetFromProto(rb.Target),
				Tracked: tracked,
			}
		}
	}
	for _, tag := range proto.LocalTags {
		v.LocalTags[tag.Name] = refTargetFromProto(tag.Target)
	}
	for _, rvProto := range proto.RemoteViews {
		rv := v.remoteView(rvProto.Name)
		for _, ref := range rvProto.Bookmarks {
			rv.Bookmarks[ref.Name] = remoteRefFromProto(ref)
		}
		for _, ref := range rvProto.Tags {
			rv.Tags[ref.Name] = remoteRefFromProto(ref)
		}
	}
	return v
}

func (v *View) remoteView(remote string) RemoteView {
	rv, ok := v.RemoteViews[remote]
	if !ok {
		rv = RemoteView{Bookmarks: map[string]RemoteRef{}, Tags: map[string]RemoteRef{}}
		v.RemoteViews[remote] = rv
	}
	return rv
}

func remoteRefFromProto(ref *pb.RemoteRef) RemoteRef {
	return RemoteRef{
		Target:  refTargetFromTerms(ref.TargetTerms),
		Tracked: ref.State == pb.RemoteRefState_Tracked,
	}
}
