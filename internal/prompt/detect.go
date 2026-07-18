package prompt

import (
	"os"
	"path/filepath"
)

// RepoType is the kind of repository found at or above a directory.
type RepoType int

// Repo types, from most to least specific.
const (
	RepoNone RepoType = iota
	RepoJJ
	RepoJJColocated
	RepoGit
)

// Detect walks up from start looking for .jj or .git.
func Detect(start string) (RepoType, string) {
	current := start
	for {
		hasJJ := isDir(filepath.Join(current, ".jj"))
		_, err := os.Stat(filepath.Join(current, ".git")) // can be a file (worktree)
		hasGit := err == nil

		switch {
		case hasJJ && hasGit:
			return RepoJJColocated, current
		case hasJJ:
			return RepoJJ, current
		case hasGit:
			return RepoGit, current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return RepoNone, ""
		}
		current = parent
	}
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
