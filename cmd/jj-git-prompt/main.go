// jj-git-prompt is a fast shell prompt segment for Jujutsu and Git
// repositories. Go port of the Rust jj-git-prompt.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/rselbach/jj-git-prompt/internal/jj"
	"github.com/rselbach/jj-git-prompt/internal/prompt"
)

var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "jj-git-prompt: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args, command := splitCommand(os.Args[1:])

	fs := flag.NewFlagSet("jj-git-prompt", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: jj-git-prompt [prompt|detect|version] [flags]") //nolint:errcheck // best-effort usage text
		fs.PrintDefaults()
	}
	var (
		cwd    = fs.String("cwd", "", "override working directory")
		bash   = fs.Bool("bash", false, "mark colors as non-printing for Bash Readline")
		noColo = fs.Bool("no-color", false, "disable output styling")

		truncateName  = fs.Int("truncate-name", -1, "max length for branch/bookmark name (0 = unlimited)")
		idLength      = fs.Int("id-length", -1, "length of change_id/commit hash to display (default 8)")
		ancestorDepth = fs.Int("ancestor-bookmark-depth", -1, "max depth to search for ancestor bookmarks (0 = disabled, default 10)")
		displayLimit  = fs.Int("bookmarks-display-limit", -1, "max bookmarks to display (0 = unlimited, default 3)")
		stripPrefix   = fs.String("strip-bookmark-prefix", "\x00", "prefixes to strip from bookmark names (comma-separated)")
		jjSymbol      = fs.String("jj-symbol", "\x00", "symbol prefix for JJ repos")
		gitSymbol     = fs.String("git-symbol", "\x00", "symbol prefix for Git repos")
		noSymbol      = fs.Bool("no-symbol", false, "disable symbol prefix entirely")

		noJJPrefix    = fs.Bool("no-jj-prefix", false, "hide \"on {symbol}\" prefix for JJ repos")
		noJJName      = fs.Bool("no-jj-name", false, "hide bookmark name for JJ repos")
		noJJID        = fs.Bool("no-jj-id", false, "hide change_id for JJ repos")
		noJJStatus    = fs.Bool("no-jj-status", false, "hide [status] for JJ repos")
		noPrefixColor = fs.Bool("no-prefix-color", false, "disable unique prefix coloring for change_id")

		noGitPrefix = fs.Bool("no-git-prefix", false, "hide \"on {symbol}\" prefix for Git repos")
		noGitName   = fs.Bool("no-git-name", false, "hide branch name for Git repos")
		noGitID     = fs.Bool("no-git-id", false, "hide (commit) for Git repos")
		noGitStatus = fs.Bool("no-git-status", false, "hide [status] for Git repos")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := prompt.NewConfig(prompt.Options{
		TruncateName:          intFlag(truncateName),
		IDLength:              intFlag(idLength),
		AncestorBookmarkDepth: intFlag(ancestorDepth),
		BookmarksDisplayLimit: intFlag(displayLimit),
		StripBookmarkPrefix:   strFlag(stripPrefix),
		JJSymbol:              strFlag(jjSymbol),
		GitSymbol:             strFlag(gitSymbol),
		NoSymbol:              *noSymbol,
		JJFlags: prompt.DisplayFlags{
			NoPrefix:      *noJJPrefix,
			NoName:        *noJJName,
			NoID:          *noJJID,
			NoStatus:      *noJJStatus,
			NoColor:       *noColo,
			NoPrefixColor: *noPrefixColor,
		},
		GitFlags: prompt.DisplayFlags{
			NoPrefix: *noGitPrefix,
			NoName:   *noGitName,
			NoID:     *noGitID,
			NoStatus: *noGitStatus,
			NoColor:  *noColo,
		},
	})

	dir := *cwd
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return err
		}
	}

	switch command {
	case "prompt":
		return runPrompt(dir, cfg, *bash)
	case "detect":
		if repoType, _ := prompt.Detect(dir); repoType == prompt.RepoNone {
			os.Exit(1)
		}
		return nil
	case "version":
		fmt.Printf("jj-git-prompt %s (go)\nfeatures: git\n", version)
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func runPrompt(dir string, cfg *prompt.Config, bash bool) error {
	var out string
	switch repoType, root := prompt.Detect(dir); repoType {
	case prompt.RepoJJ, prompt.RepoJJColocated:
		info, err := jj.Collect(root, cfg.IDLength, cfg.AncestorBookmarkDepth)
		if err != nil {
			return err
		}
		out = prompt.FormatJJ(info, cfg)
	case prompt.RepoGit:
		info, err := prompt.CollectGit(root, cfg.IDLength)
		if err != nil {
			return err
		}
		out = prompt.FormatGit(info, cfg)
	default:
		return nil
	}
	if bash {
		out = prompt.EscapeForBash(out)
	}
	fmt.Print(out)
	return nil
}

// valueFlags are the flags that consume the following argument when given
// in "--flag value" form.
var valueFlags = map[string]bool{
	"cwd": true, "truncate-name": true, "id-length": true,
	"ancestor-bookmark-depth": true, "bookmarks-display-limit": true,
	"strip-bookmark-prefix": true, "jj-symbol": true, "git-symbol": true,
}

// splitCommand extracts the optional subcommand (the first argument that is
// neither a flag nor a flag value), leaving the flags for the flag set.
// Defaults to "prompt".
func splitCommand(args []string) ([]string, string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) == 0 || arg[0] != '-' {
			return slices.Concat(args[:i], args[i+1:]), arg
		}
		name := strings.TrimLeft(arg, "-")
		if !strings.Contains(name, "=") && valueFlags[name] {
			i++ // skip the flag's value
		}
	}
	return args, "prompt"
}

// intFlag converts the -1 sentinel back to "not set".
func intFlag(v *int) *int {
	if *v == -1 {
		return nil
	}
	return v
}

// strFlag converts the \x00 sentinel back to "not set".
func strFlag(v *string) *string {
	if *v == "\x00" {
		return nil
	}
	return v
}
