package jj

import (
	pb "github.com/rselbach/jj-git-prompt/internal/jj/pb/simple_op_storepb"
)

// RefTarget mirrors jj's RefTarget: a Merge of optional commit ids stored in
// interleaved order [add0, remove0, add1, remove1, ..., addN]. A nil term
// means "absent". A normal (non-conflicting) target has a single term.
type RefTarget struct {
	terms []*CommitID
}

// absentRefTarget is a single absent term, jj's RefTarget::absent().
func absentRefTarget() RefTarget {
	return RefTarget{terms: []*CommitID{nil}}
}

// IsAbsent reports whether the target is a resolved absence.
func (t RefTarget) IsAbsent() bool {
	return len(t.terms) == 1 && t.terms[0] == nil
}

// AsNormal returns the single commit id if the target is resolved and
// present.
func (t RefTarget) AsNormal() (CommitID, bool) {
	if len(t.terms) == 1 && t.terms[0] != nil {
		return *t.terms[0], true
	}
	return "", false
}

// AddedIDs returns the positive (even-indexed) non-absent terms.
func (t RefTarget) AddedIDs() []CommitID {
	var ids []CommitID
	for i := 0; i < len(t.terms); i += 2 {
		if t.terms[i] != nil {
			ids = append(ids, *t.terms[i])
		}
	}
	return ids
}

// Equal reports term-wise equality, matching Merge's PartialEq in Rust.
func (t RefTarget) Equal(o RefTarget) bool {
	if len(t.terms) != len(o.terms) {
		return false
	}
	for i, a := range t.terms {
		b := o.terms[i]
		switch {
		case a == nil && b == nil:
		case a == nil || b == nil:
			return false
		case *a != *b:
			return false
		}
	}
	return true
}

// refTargetFromProto decodes the RefTarget proto including its legacy forms,
// mirroring ref_target_from_proto in simple_op_store.rs.
func refTargetFromProto(proto *pb.RefTarget) RefTarget {
	if proto == nil {
		return absentRefTarget()
	}
	switch v := proto.Value.(type) {
	case *pb.RefTarget_CommitId:
		id := CommitID(v.CommitId) //nolint:staticcheck // legacy field read for pre-0.9 repos, as jj-lib does
		return RefTarget{terms: []*CommitID{&id}}
	case *pb.RefTarget_ConflictLegacy:
		return refTargetFromLegacy(v.ConflictLegacy.Removes, v.ConflictLegacy.Adds) //nolint:staticcheck // legacy field
	case *pb.RefTarget_Conflict:
		terms := make([]*CommitID, 0, len(v.Conflict.Adds)+len(v.Conflict.Removes))
		for i, add := range v.Conflict.Adds {
			terms = append(terms, optCommitID(add.Value))
			if i < len(v.Conflict.Removes) {
				terms = append(terms, optCommitID(v.Conflict.Removes[i].Value))
			}
		}
		return RefTarget{terms: terms}
	default:
		return absentRefTarget()
	}
}

// refTargetFromLegacy mirrors Merge::from_legacy_form: pad removes with
// absent terms until adds.len() == removes.len() + 1, then interleave.
func refTargetFromLegacy(removes, adds [][]byte) RefTarget {
	numRemoves := len(removes)
	if len(adds) > 0 {
		numRemoves = max(numRemoves, len(adds)-1)
	}
	terms := make([]*CommitID, 0, 2*numRemoves+1)
	term := func(list [][]byte, i int) *CommitID {
		if i >= len(list) {
			return nil
		}
		id := CommitID(list[i])
		return &id
	}
	for i := 0; i < numRemoves; i++ {
		terms = append(terms, term(adds, i), term(removes, i))
	}
	terms = append(terms, term(adds, numRemoves))
	return RefTarget{terms: terms}
}

// refTargetFromTerms decodes the native alternating term list used by
// RemoteRef.
func refTargetFromTerms(terms []*pb.RefTargetTerm) RefTarget {
	if len(terms) == 0 {
		return absentRefTarget()
	}
	out := make([]*CommitID, len(terms))
	for i, term := range terms {
		out[i] = optCommitID(term.Value)
	}
	return RefTarget{terms: out}
}

func optCommitID(b []byte) *CommitID {
	if b == nil {
		return nil
	}
	id := CommitID(b)
	return &id
}
