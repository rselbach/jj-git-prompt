package jj

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"google.golang.org/protobuf/proto"

	gspb "github.com/rselbach/jj-git-prompt/internal/jj/pb/git_storepb"
)

// emptyTreeID is git's well-known empty tree object id (SHA-1).
const emptyTreeID = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

const commitIDLength = 20 // SHA-1; jj's SHA-256 git support is still experimental

// Commit is the subset of jj's backend::Commit the prompt needs.
type Commit struct {
	ID          CommitID
	Parents     []CommitID
	ChangeID    ChangeID
	Description string
	// RootTree holds the root tree merge terms (tree object ids). A single
	// term means the commit is conflict-free.
	RootTree []string
}

// HasConflict reports whether the commit's tree is an unresolved merge,
// mirroring Commit::has_conflict.
func (c *Commit) HasConflict() bool { return len(c.RootTree) > 1 }

// gitBackend reads jj commits stored as git commits plus extra metadata.
type gitBackend struct {
	objects  *filesystem.ObjectStorage
	storeDir string

	extrasOnce sync.Once
	extrasErr  error
	extras     []*stackedTable
}

// openGitBackend resolves .jj/repo/store/git_target and opens the git
// object database it points to.
func openGitBackend(repoPath string) (*gitBackend, error) {
	storeDir := filepath.Join(repoPath, "store")
	target, err := os.ReadFile(filepath.Join(storeDir, "git_target"))
	if err != nil {
		return nil, fmt.Errorf("read git_target: %w", err)
	}
	gitDir := string(target)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(storeDir, gitDir)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "objects")); err != nil {
		return nil, fmt.Errorf("open git objects %s: %w", gitDir, err)
	}
	// JJ supplies commit IDs from its own view and index, so we only need
	// objects. Opening a full go-git repository also validates ref storage,
	// rejecting reftable even though it does not change the object format.
	storage := filesystem.NewStorage(osfs.New(gitDir), cache.NewObjectLRUDefault())
	cfg, err := storage.Config()
	if err != nil {
		return nil, fmt.Errorf("read git config %s: %w", gitDir, err)
	}
	if format := cfg.Extensions.ObjectFormat; format != "" && format != "sha1" {
		return nil, fmt.Errorf("unsupported git object format %q", format)
	}
	return &gitBackend{objects: &storage.ObjectStorage, storeDir: storeDir}, nil
}

// extrasTables lazily loads the extra-metadata stacked tables. Old commits
// (before jj started writing change-id headers in 2024) have their change
// ids only there.
func (b *gitBackend) extrasTables() []*stackedTable {
	b.extrasOnce.Do(func() {
		dir := filepath.Join(b.storeDir, "extra")
		b.extras, b.extrasErr = loadStackedTables(dir, commitIDLength)
	})
	return b.extras
}

// ReadCommit mirrors GitBackend::read_commit, minus the write-path import
// of missing extras: commits absent from the extras table fall back to the
// change-id commit header or a synthetic change id.
func (b *gitBackend) ReadCommit(id CommitID) (*Commit, error) {
	if id.IsRoot() {
		return rootCommit(id), nil
	}

	raw, err := b.rawCommit(id)
	if err != nil {
		return nil, err
	}
	commit, err := parseGitCommit(id, raw)
	if err != nil {
		return nil, err
	}
	if len(commit.Parents) == 0 {
		commit.Parents = []CommitID{rootCommitID(len(id))}
	}

	for _, table := range b.extrasTables() {
		extras, ok := table.get([]byte(id))
		if !ok {
			continue
		}
		applyExtras(commit, extras)
		break
	}
	if commit.ChangeID == "" {
		commit.ChangeID = syntheticChangeID(id)
	}
	return commit, nil
}

func rootCommit(id CommitID) *Commit {
	empty, _ := hex.DecodeString(emptyTreeID)
	return &Commit{
		ID:       id,
		ChangeID: ChangeID(make([]byte, changeIDLength)),
		RootTree: []string{string(empty)},
	}
}

func (b *gitBackend) rawCommit(id CommitID) ([]byte, error) {
	hash := plumbing.NewHash(id.Hex())
	obj, err := b.objects.EncodedObject(plumbing.CommitObject, hash)
	if err != nil {
		return nil, fmt.Errorf("read git commit %s: %w", id.Hex(), err)
	}
	r, err := obj.Reader()
	if err != nil {
		return nil, fmt.Errorf("read git commit %s: %w", id.Hex(), err)
	}
	defer r.Close() //nolint:errcheck // read-only reader
	return io.ReadAll(r)
}

// parseGitCommit extracts parents, tree, jj headers, and the message from a
// raw commit object. Header continuation lines (leading space) belong to
// the preceding header.
func parseGitCommit(id CommitID, raw []byte) (*Commit, error) {
	commit := &Commit{ID: id}
	var treeHex string
	var jjTrees string

	br := bufio.NewReader(bytes.NewReader(raw))
	for {
		line, err := br.ReadString('\n')
		if err != nil && line == "" {
			break
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			msg, _ := io.ReadAll(br)
			commit.Description = string(msg)
			break
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		switch key {
		case "tree":
			treeHex = value
		case "parent":
			parent, err := hex.DecodeString(value)
			if err != nil {
				return nil, fmt.Errorf("commit %s: bad parent %q", id.Hex(), value)
			}
			commit.Parents = append(commit.Parents, CommitID(parent))
		case "change-id":
			if decoded, ok := DecodeReverseHex(value); ok && len(decoded) == changeIDLength {
				commit.ChangeID = ChangeID(decoded)
			}
		case "jj:trees":
			jjTrees = value
		}
	}

	if err := setRootTree(commit, treeHex, jjTrees); err != nil {
		return nil, fmt.Errorf("commit %s: %w", id.Hex(), err)
	}
	return commit, nil
}

// setRootTree mirrors extract_root_tree_from_commit: the jj:trees header
// holds space-separated tree ids of a conflicted tree; otherwise the git
// tree is the resolved root tree.
func setRootTree(commit *Commit, treeHex, jjTrees string) error {
	if jjTrees == "" {
		tree, err := hex.DecodeString(treeHex)
		if err != nil {
			return fmt.Errorf("bad tree %q", treeHex)
		}
		commit.RootTree = []string{string(tree)}
		return nil
	}
	for _, h := range strings.Split(jjTrees, " ") {
		tree, err := hex.DecodeString(h)
		if err != nil || len(tree) != commitIDLength {
			return fmt.Errorf("bad jj:trees %q", jjTrees)
		}
		commit.RootTree = append(commit.RootTree, string(tree))
	}
	if len(commit.RootTree) == 1 || len(commit.RootTree)%2 == 0 {
		return fmt.Errorf("invalid jj:trees %q", jjTrees)
	}
	return nil
}

// applyExtras mirrors deserialize_extras for the fields we keep.
func applyExtras(commit *Commit, extras []byte) {
	var extrasProto gspb.Commit
	if err := proto.Unmarshal(extras, &extrasProto); err != nil {
		return
	}
	if len(extrasProto.ChangeId) > 0 {
		commit.ChangeID = ChangeID(extrasProto.ChangeId)
	}
	if len(commit.RootTree) == 1 && extrasProto.UsesTreeConflictFormat && len(extrasProto.RootTree) > 0 {
		commit.RootTree = commit.RootTree[:0]
		for _, tree := range extrasProto.RootTree {
			commit.RootTree = append(commit.RootTree, string(tree))
		}
	}
}

// syntheticChangeID mirrors synthetic_change_id_from_git_commit_id: the
// last 16 bytes of the commit id, reversed, with each byte's bits reversed.
func syntheticChangeID(id CommitID) ChangeID {
	bytes := []byte(id)[len(id)-changeIDLength:]
	out := make([]byte, changeIDLength)
	for i, b := range bytes {
		out[changeIDLength-1-i] = reverseBits(b)
	}
	return ChangeID(out)
}

func reverseBits(b byte) byte {
	b = b>>4 | b<<4
	b = b>>2&0x33 | b<<2&0xcc
	b = b>>1&0x55 | b<<1&0xaa
	return b
}
