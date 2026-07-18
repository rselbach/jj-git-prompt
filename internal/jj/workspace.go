package jj

import (
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	lwcpb "github.com/rselbach/jj-git-prompt/internal/jj/pb/local_working_copypb"
)

// Workspace locates the .jj metadata for a workspace root.
type Workspace struct {
	// Root is the workspace root (parent of .jj).
	Root string
	// RepoPath is the .jj/repo directory (possibly redirected for secondary
	// workspaces).
	RepoPath string
	// Name is the workspace name from the working-copy checkout state.
	Name string
}

// LoadWorkspace mirrors DefaultWorkspaceLoader: resolves .jj/repo (a
// directory, or a file containing a relative path to the repo directory of
// the primary workspace) and reads the workspace name from
// .jj/working_copy/checkout.
func LoadWorkspace(root string) (*Workspace, error) {
	jjDir := filepath.Join(root, ".jj")
	if fi, err := os.Stat(jjDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("no jj workspace at %s", root)
	}

	repoDir := filepath.Join(jjDir, "repo")
	if fi, err := os.Stat(repoDir); err == nil && !fi.IsDir() {
		buf, err := os.ReadFile(repoDir)
		if err != nil {
			return nil, fmt.Errorf("read repo path: %w", err)
		}
		repoDir = filepath.Join(jjDir, string(buf))
		if fi, err := os.Stat(repoDir); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("repo directory %s does not exist", repoDir)
		}
	}

	name, err := workspaceName(filepath.Join(jjDir, "working_copy"))
	if err != nil {
		return nil, err
	}
	return &Workspace{Root: root, RepoPath: repoDir, Name: name}, nil
}

func workspaceName(stateDir string) (string, error) {
	buf, err := os.ReadFile(filepath.Join(stateDir, "checkout"))
	if err != nil {
		return "", fmt.Errorf("read checkout state: %w", err)
	}
	var checkout lwcpb.Checkout
	if err := proto.Unmarshal(buf, &checkout); err != nil {
		return "", fmt.Errorf("decode checkout state: %w", err)
	}
	if checkout.WorkspaceName == "" {
		return "default", nil
	}
	return checkout.WorkspaceName, nil
}
