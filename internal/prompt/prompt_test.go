package prompt

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rselbach/jj-git-prompt/internal/jj"
)

func allVisible() DisplayConfig {
	return DisplayConfig{
		ShowPrefix: true, ShowName: true, ShowID: true,
		ShowStatus: true, ShowColor: true, ShowPrefixColor: true,
	}
}

func testConfig() *Config {
	return &Config{
		IDLength:              8,
		AncestorBookmarkDepth: 10,
		JJDisplay:             allVisible(),
		GitDisplay:            allVisible(),
	}
}

func TestEscapeForBash(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"color sequences": {
			input: "on \x1b[34mJJ\x1b[0m",
			want:  "on \x01\x1b[34m\x02JJ\x01\x1b[0m\x02",
		},
		"plain text": {input: "on JJ", want: "on JJ"},
		"incomplete": {input: "on \x1b[34", want: "on \x1b[34"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(tc.want, EscapeForBash(tc.input))
		})
	}
}

func TestFormatJJ(t *testing.T) {
	tests := map[string]struct {
		info jj.Info
		cfg  func(*Config)
		want string
	}{
		"clean with bookmark": {
			info: jj.Info{
				ChangeID: "yzxv1234", ChangeIDPrefixLen: 4,
				Bookmarks: []jj.Bookmark{{Name: "main"}},
				HasRemote: true, IsSynced: true,
			},
			want: "on \x1b[34m\x1b[0m\x1b[95myzxv\x1b[0m\x1b[90m1234\x1b[0m \x1b[32m(main)\x1b[0m",
		},
		"conflict and empty desc": {
			info: jj.Info{
				ChangeID: "yzxv1234", ChangeIDPrefixLen: 4,
				EmptyDesc: true, Conflict: true, IsSynced: true,
			},
			want: "on \x1b[34m\x1b[0m\x1b[95myzxv\x1b[0m\x1b[90m1234\x1b[0m \x1b[31m[!∅]\x1b[0m",
		},
		"empty desc only is yellow": {
			info: jj.Info{
				ChangeID: "yzxv1234", ChangeIDPrefixLen: 4,
				EmptyDesc: true, IsSynced: true,
			},
			want: "on \x1b[34m\x1b[0m\x1b[95myzxv\x1b[0m\x1b[90m1234\x1b[0m \x1b[33m[∅]\x1b[0m",
		},
		"display limit with overflow": {
			info: jj.Info{
				ChangeID: "yzxv1234", ChangeIDPrefixLen: 4,
				Bookmarks: []jj.Bookmark{
					{Name: "main"}, {Name: "feat/foo", Distance: 1},
					{Name: "feat/bar", Distance: 2}, {Name: "staging", Distance: 3},
					{Name: "develop", Distance: 4},
				},
				IsSynced: true,
			},
			cfg:  func(c *Config) { c.BookmarksDisplayLimit = 2 },
			want: "on \x1b[34m\x1b[0m\x1b[95myzxv\x1b[0m\x1b[90m1234\x1b[0m \x1b[32m(main, feat/foo~1, …+3)\x1b[0m",
		},
		"strip prefix and truncate": {
			info: jj.Info{
				ChangeID: "yzxv1234", ChangeIDPrefixLen: 4,
				Bookmarks: []jj.Bookmark{{Name: "dmmulroy/very-long-feature-name"}},
				IsSynced:  true,
			},
			cfg: func(c *Config) {
				c.StripBookmarkPrefix = []string{"dmmulroy/"}
				c.TruncateName = 10
			},
			want: "on \x1b[34m\x1b[0m\x1b[95myzxv\x1b[0m\x1b[90m1234\x1b[0m \x1b[32m(very-long…)\x1b[0m",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig()
			if tc.cfg != nil {
				tc.cfg(cfg)
			}
			require.New(t).Equal(tc.want, FormatJJ(&tc.info, cfg))
		})
	}
}

func TestParseGitStatus(t *testing.T) {
	tests := map[string]struct {
		out  string
		want GitInfo
	}{
		"clean with upstream": {
			out: "# branch.oid 9f97b71c1111222233334444555566667777aaaa\n" +
				"# branch.head main\n" +
				"# branch.upstream origin/main\n" +
				"# branch.ab +2 -1\n",
			want: GitInfo{Branch: "main", HeadShort: "9f97b71c", Ahead: 2, Behind: 1},
		},
		"detached dirty": {
			out: "# branch.oid 9f97b71c1111222233334444555566667777aaaa\n" +
				"# branch.head (detached)\n" +
				"1 .M N... 100644 100644 100644 aaaa bbbb a.txt\n" +
				"1 M. N... 100644 100644 100644 aaaa bbbb b.txt\n" +
				"1 .D N... 100644 100644 100644 aaaa bbbb c.txt\n" +
				"u UU N... 100644 100644 100644 100644 aaaa bbbb cccc d.txt\n" +
				"? new.txt\n",
			want: GitInfo{
				HeadShort: "9f97b71c", Staged: 1, Modified: 1,
				Deleted: 1, Conflicted: 1, Untracked: 1,
			},
		},
		"empty repo": {
			out:  "# branch.oid (initial)\n# branch.head main\n",
			want: GitInfo{Branch: "main", HeadShort: "empty"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(&tc.want, parseGitStatus(tc.out, 8))
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := map[string]struct {
		limit int
		input string
		want  string
	}{
		"unlimited":    {limit: 0, input: "very-long-name", want: "very-long-name"},
		"within limit": {limit: 20, input: "short", want: "short"},
		"truncated":    {limit: 5, input: "very-long-name", want: "very…"},
		"limit one":    {limit: 1, input: "ab", want: "…"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{TruncateName: tc.limit}
			require.New(t).Equal(tc.want, cfg.Truncate(tc.input))
		})
	}
}
