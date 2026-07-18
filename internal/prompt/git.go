package prompt

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitInfo is the git repository status for the prompt.
type GitInfo struct {
	// Branch is empty when HEAD is detached.
	Branch     string
	HeadShort  string
	Staged     int
	Modified   int
	Untracked  int
	Deleted    int
	Conflicted int
	Ahead      int
	Behind     int
}

// CollectGit gathers git status by running `git status --porcelain=v2`.
// The Rust version links libgit2; shelling out to git keeps status fast on
// large repos (git's index caching) without a C dependency.
func CollectGit(root string, idLength int) (*GitInfo, error) {
	out, err := exec.Command("git", "-C", root, "--no-optional-locks",
		"status", "--porcelain=v2", "--branch").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("git status: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git status: %w", err)
	}
	return parseGitStatus(string(out), idLength), nil
}

func parseGitStatus(out string, idLength int) *GitInfo {
	info := &GitInfo{}
	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			oid := strings.TrimPrefix(line, "# branch.oid ")
			if oid == "(initial)" {
				info.HeadShort = "empty"
				continue
			}
			info.HeadShort = oid[:min(idLength, len(oid))]
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head != "(detached)" {
				info.Branch = head
			}
		case strings.HasPrefix(line, "# branch.ab "):
			// Malformed counts leave ahead/behind at zero.
			_, _ = fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &info.Ahead, &info.Behind)
		case strings.HasPrefix(line, "u "):
			info.Conflicted++
		case strings.HasPrefix(line, "? "):
			info.Untracked++
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			if len(line) < 4 {
				continue
			}
			// XY: X = staged (index) state, Y = working-tree state.
			x, y := line[2], line[3]
			if x != '.' {
				info.Staged++
			}
			switch y {
			case 'M', 'T':
				info.Modified++
			case 'D':
				info.Deleted++
			}
		}
	}
	return info
}
