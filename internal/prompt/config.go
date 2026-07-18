// Package prompt implements repo detection, info collection, and prompt
// formatting for jj-git-prompt.
package prompt

import (
	"os"
	"strconv"
	"strings"
)

// Default symbols for repo prefixes.
const (
	DefaultJJSymbol  = "󱗆 "
	DefaultGitSymbol = " "
)

// DisplayConfig holds the visibility toggles for one repo type.
type DisplayConfig struct {
	ShowPrefix bool
	ShowName   bool
	ShowID     bool
	ShowStatus bool
	ShowColor  bool
	// ShowPrefixColor enables unique-prefix coloring of the change id (jj
	// only).
	ShowPrefixColor bool
}

// Config holds all prompt options.
type Config struct {
	// TruncateName is the max bookmark/branch name length (0 = unlimited).
	TruncateName int
	// IDLength is the displayed change id / commit hash length.
	IDLength int
	// AncestorBookmarkDepth is the max ancestor search depth (0 = disabled).
	AncestorBookmarkDepth int
	// BookmarksDisplayLimit caps displayed bookmarks (0 = unlimited).
	BookmarksDisplayLimit int
	// StripBookmarkPrefix lists prefixes stripped from bookmark names.
	StripBookmarkPrefix []string
	JJSymbol            string
	GitSymbol           string
	JJDisplay           DisplayConfig
	GitDisplay          DisplayConfig
}

// DisplayFlags is the negated CLI form of DisplayConfig.
type DisplayFlags struct {
	NoPrefix      bool
	NoName        bool
	NoID          bool
	NoStatus      bool
	NoColor       bool
	NoPrefixColor bool
}

func (f DisplayFlags) intoConfig(envPrefix string) DisplayConfig {
	return DisplayConfig{
		ShowPrefix:      !f.NoPrefix && !envSet(envPrefix+"_PREFIX"),
		ShowName:        !f.NoName && !envSet(envPrefix+"_NAME"),
		ShowID:          !f.NoID && !envSet(envPrefix+"_ID"),
		ShowStatus:      !f.NoStatus && !envSet(envPrefix+"_STATUS"),
		ShowColor:       !f.NoColor && !envSet(envPrefix+"_COLOR"),
		ShowPrefixColor: !f.NoPrefixColor && !envSet("JJ_GIT_PROMPT_NO_PREFIX_COLOR"),
	}
}

func envSet(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}

// Options are the raw CLI values used to build a Config; nil means the flag
// was not given and the environment (then default) applies.
type Options struct {
	TruncateName          *int
	IDLength              *int
	AncestorBookmarkDepth *int
	BookmarksDisplayLimit *int
	StripBookmarkPrefix   *string
	JJSymbol              *string
	GitSymbol             *string
	NoSymbol              bool
	JJFlags               DisplayFlags
	GitFlags              DisplayFlags
}

// NewConfig builds a Config from CLI options and environment variables; CLI
// takes precedence.
func NewConfig(opts Options) *Config {
	cfg := &Config{
		TruncateName:          intOption(opts.TruncateName, "JJ_GIT_PROMPT_TRUNCATE_NAME", 0),
		IDLength:              intOption(opts.IDLength, "JJ_GIT_PROMPT_ID_LENGTH", 8),
		AncestorBookmarkDepth: intOption(opts.AncestorBookmarkDepth, "JJ_GIT_PROMPT_ANCESTOR_BOOKMARK_DEPTH", 10),
		BookmarksDisplayLimit: intOption(opts.BookmarksDisplayLimit, "JJ_GIT_PROMPT_BOOKMARKS_DISPLAY_LIMIT", 3),
		JJDisplay:             opts.JJFlags.intoConfig("JJ_GIT_PROMPT_NO_JJ"),
		GitDisplay:            opts.GitFlags.intoConfig("JJ_GIT_PROMPT_NO_GIT"),
	}
	if s := strOption(opts.StripBookmarkPrefix, "JJ_GIT_PROMPT_STRIP_BOOKMARK_PREFIX", ""); s != "" {
		cfg.StripBookmarkPrefix = strings.Split(s, ",")
	}
	if !opts.NoSymbol {
		cfg.JJSymbol = strOption(opts.JJSymbol, "JJ_GIT_PROMPT_JJ_SYMBOL", DefaultJJSymbol)
		cfg.GitSymbol = strOption(opts.GitSymbol, "JJ_GIT_PROMPT_GIT_SYMBOL", DefaultGitSymbol)
	}
	return cfg
}

func intOption(cli *int, env string, def int) int {
	if cli != nil {
		return *cli
	}
	if v, err := strconv.Atoi(os.Getenv(env)); err == nil {
		return v
	}
	return def
}

func strOption(cli *string, env, def string) string {
	if cli != nil {
		return *cli
	}
	if v, ok := os.LookupEnv(env); ok {
		return v
	}
	return def
}

// Truncate shortens s to TruncateName runes with an ellipsis.
func (c *Config) Truncate(s string) string {
	if c.TruncateName == 0 {
		return s
	}
	runes := []rune(s)
	switch {
	case len(runes) <= c.TruncateName:
		return s
	case c.TruncateName <= 1:
		return "…"
	default:
		return string(runes[:c.TruncateName-1]) + "…"
	}
}

// StripPrefix removes the first matching configured prefix from s.
func (c *Config) StripPrefix(s string) string {
	for _, prefix := range c.StripBookmarkPrefix {
		if stripped, ok := strings.CutPrefix(s, prefix); ok {
			return stripped
		}
	}
	return s
}
