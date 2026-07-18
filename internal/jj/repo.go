package jj

import "fmt"

// Repo is a loaded read-only jj repository at its current operation head.
type Repo struct {
	Workspace *Workspace
	View      *View

	backend *gitBackend
	// index may be nil when no usable index link exists; callers degrade
	// (no divergence detection, fallback prefix length).
	index *Index
}

// Load opens the workspace at root and loads the view at the current
// operation head. Purely read-only: divergent op heads are resolved in
// memory and missing indexes are tolerated rather than rebuilt.
func Load(root string) (*Repo, error) {
	ws, err := LoadWorkspace(root)
	if err != nil {
		return nil, err
	}
	op, err := headOperation(ws.RepoPath)
	if err != nil {
		return nil, err
	}
	view, err := readView(ws.RepoPath, op.ViewID)
	if err != nil {
		return nil, err
	}
	backend, err := openGitBackend(ws.RepoPath)
	if err != nil {
		return nil, err
	}
	index, err := loadIndexAtOperation(ws.RepoPath, op)
	if err != nil {
		return nil, err
	}
	return &Repo{Workspace: ws, View: view, backend: backend, index: index}, nil
}

// WCCommitID returns the working-copy commit id for this workspace.
func (r *Repo) WCCommitID() (CommitID, error) {
	id, ok := r.View.WCCommitIDs[r.Workspace.Name]
	if !ok {
		return "", fmt.Errorf("no working copy for workspace %q", r.Workspace.Name)
	}
	return id, nil
}

// ReadCommit reads a commit from the git backend.
func (r *Repo) ReadCommit(id CommitID) (*Commit, error) {
	return r.backend.ReadCommit(id)
}

// parentIDs returns a commit's parent ids, preferring the index over git
// object reads.
func (r *Repo) parentIDs(id CommitID) ([]CommitID, error) {
	if r.index != nil {
		if pos, ok := r.index.CommitPos(id); ok {
			positions := r.index.ParentPositions(pos)
			parents := make([]CommitID, len(positions))
			for i, p := range positions {
				parents[i] = r.index.CommitIDAt(p)
			}
			return parents, nil
		}
	}
	commit, err := r.backend.ReadCommit(id)
	if err != nil {
		return nil, err
	}
	return commit.Parents, nil
}
