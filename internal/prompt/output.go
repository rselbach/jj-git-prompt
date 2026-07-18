package prompt

import (
	"fmt"
	"strings"

	"github.com/rselbach/jj-git-prompt/internal/jj"
)

// ANSI SGR sequences; standard colors so they adapt to the terminal theme.
const (
	colorReset         = "\x1b[0m"
	colorPurple        = "\x1b[35m"
	colorGreen         = "\x1b[32m"
	colorRed           = "\x1b[31m"
	colorBlue          = "\x1b[34m"
	colorYellow        = "\x1b[33m"
	colorBrightMagenta = "\x1b[95m"
	colorBrightBlack   = "\x1b[90m"
)

// EscapeForBash wraps ANSI SGR sequences in \x01/\x02 so Bash Readline
// treats them as non-printing.
func EscapeForBash(input string) string {
	var out strings.Builder
	out.Grow(len(input))
	for {
		start := strings.Index(input, "\x1b[")
		if start < 0 {
			out.WriteString(input)
			return out.String()
		}
		out.WriteString(input[:start])
		seq := input[start+2:]
		end := strings.IndexByte(seq, 'm')
		if end < 0 {
			out.WriteString(input[start:])
			return out.String()
		}
		out.WriteString("\x01\x1b[")
		out.WriteString(seq[:end+1])
		out.WriteString("\x02")
		input = seq[end+1:]
	}
}

func segment(text, color string, showColor bool) string {
	if !showColor {
		return text
	}
	return color + text + colorReset
}

// formatChangeID highlights the unique prefix like jj log does: bright
// magenta prefix, gray rest.
func formatChangeID(changeID string, prefixLen int) string {
	prefixLen = min(prefixLen, len(changeID))
	prefix, rest := changeID[:prefixLen], changeID[prefixLen:]
	if rest == "" {
		return colorBrightMagenta + prefix + colorReset
	}
	return colorBrightMagenta + prefix + colorReset + colorBrightBlack + rest + colorReset
}

// FormatJJ renders `on {symbol}{change_id} ({bookmarks}) [{status}]`.
func FormatJJ(info *jj.Info, cfg *Config) string {
	var out strings.Builder
	display := cfg.JJDisplay

	if display.ShowPrefix {
		out.WriteString("on ")
		out.WriteString(segment(cfg.JJSymbol, colorBlue, display.ShowColor))
	}

	if display.ShowID {
		if display.ShowColor && display.ShowPrefixColor {
			out.WriteString(formatChangeID(info.ChangeID, info.ChangeIDPrefixLen))
		} else {
			out.WriteString(segment(info.ChangeID, colorPurple, display.ShowColor))
		}
	}

	if display.ShowName && len(info.Bookmarks) > 0 {
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(segment(formatBookmarks(info.Bookmarks, cfg), colorGreen, display.ShowColor))
	}

	if display.ShowStatus {
		var status strings.Builder
		if info.Conflict {
			status.WriteRune('!')
		}
		if info.Divergent {
			status.WriteRune('⇔')
		}
		// Only flag missing descriptions on commits with actual changes.
		if info.EmptyDesc && !info.EmptyCommit {
			status.WriteRune('∅')
		}
		if info.HasRemote && !info.IsSynced {
			status.WriteRune('⇡')
		}
		if status.Len() > 0 {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			color := colorRed
			if status.String() == "∅" {
				color = colorYellow
			}
			out.WriteString(segment("["+status.String()+"]", color, display.ShowColor))
		}
	}

	return out.String()
}

func formatBookmarks(bookmarks []jj.Bookmark, cfg *Config) string {
	total := len(bookmarks)
	show := total
	if cfg.BookmarksDisplayLimit > 0 {
		show = min(cfg.BookmarksDisplayLimit, total)
	}

	parts := make([]string, 0, show+1)
	for _, bm := range bookmarks[:show] {
		name := cfg.Truncate(cfg.StripPrefix(bm.Name))
		if bm.Distance > 0 {
			name = fmt.Sprintf("%s~%d", name, bm.Distance)
		}
		parts = append(parts, name)
	}
	if hidden := total - show; hidden > 0 {
		parts = append(parts, fmt.Sprintf("…+%d", hidden))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// FormatGit renders `on {symbol}{name} ({id}) [{status}]`.
func FormatGit(info *GitInfo, cfg *Config) string {
	var out strings.Builder
	display := cfg.GitDisplay

	if display.ShowPrefix {
		out.WriteString("on ")
		out.WriteString(segment(cfg.GitSymbol, colorBlue, display.ShowColor))
	}

	if display.ShowName {
		name := "HEAD"
		if info.Branch != "" {
			name = cfg.Truncate(info.Branch)
		}
		out.WriteString(segment(name, colorPurple, display.ShowColor))
	}

	if display.ShowID {
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(segment("("+info.HeadShort+")", colorGreen, display.ShowColor))
	}

	if display.ShowStatus {
		var status strings.Builder
		if info.Conflicted > 0 {
			status.WriteRune('=')
		}
		if info.Staged > 0 {
			status.WriteRune('+')
		}
		if info.Modified > 0 {
			status.WriteRune('!')
		}
		if info.Untracked > 0 {
			status.WriteRune('?')
		}
		if info.Deleted > 0 {
			status.WriteRune('✘')
		}
		if info.Ahead > 0 {
			fmt.Fprintf(&status, "⇡%d", info.Ahead)
		}
		if info.Behind > 0 {
			fmt.Fprintf(&status, "⇣%d", info.Behind)
		}
		if status.Len() > 0 {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(segment("["+status.String()+"]", colorRed, display.ShowColor))
		}
	}

	return out.String()
}
