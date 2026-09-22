package jj

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitBackendRefStorage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for name, tc := range map[string]struct {
		refStorage string
		packed     bool
	}{
		"files loose":     {refStorage: "files"},
		"files packed":    {refStorage: "files", packed: true},
		"reftable loose":  {refStorage: "reftable"},
		"reftable packed": {refStorage: "reftable", packed: true},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", out)
				return strings.TrimSpace(string(out))
			}
			git("init", "--object-format=sha1", "--ref-format="+tc.refStorage)
			require.NoError(t, os.WriteFile(filepath.Join(root, "greendale.txt"), []byte("Troy Barnes\n"), 0o600))
			git("add", "greendale.txt")
			git("-c", "user.name=Troy Barnes", "-c", "user.email=troy@greendale.example", "-c", "commit.gpgsign=false", "commit", "-m", "Greendale")
			id, err := hex.DecodeString(git("rev-parse", "HEAD"))
			require.NoError(t, err)
			if tc.packed {
				git("repack", "-ad")
				git("prune-packed")
			}

			repoPath := filepath.Join(root, ".jj", "repo")
			storeDir := filepath.Join(repoPath, "store")
			require.NoError(t, os.MkdirAll(storeDir, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(storeDir, "git_target"), []byte("../../../.git"), 0o600))
			backend, err := openGitBackend(repoPath)
			require.NoError(t, err)
			commit, err := backend.ReadCommit(CommitID(id))
			require.NoError(t, err)
			require.Equal(t, "Greendale\n", commit.Description)
			require.Len(t, commit.RootTree, 1)
			entries, err := backend.readTree(commit.RootTree[0])
			require.NoError(t, err)
			require.Contains(t, entries, "greendale.txt")
		})
	}
}
