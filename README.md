# jj-git-prompt

A fast, unified shell prompt segment for [Jujutsu](https://jj-vcs.github.io/jj/)
and Git repositories. It detects either repository type from any subdirectory
and renders the relevant revision, bookmark or branch, and working-copy
status.

This project is inspired by
[dmmulroy/jj-starship](https://github.com/dmmulroy/jj-starship). It brings the
same unified JJ and Git prompt approach to a standalone Go binary. JJ metadata
is read directly from its on-disk files; Git status is collected with the Git
CLI.

## Installation

Install the latest version with Go:

```sh
go install github.com/rselbach/jj-git-prompt/cmd/jj-git-prompt@latest
```

Prebuilt binaries are also available from the latest GitHub release:

| Platform | Binary |
| --- | --- |
| Linux x64 (`amd64`) | [`jj-git-prompt-linux-amd64`](https://github.com/rselbach/jj-git-prompt/releases/latest/download/jj-git-prompt-linux-amd64) |
| Linux ARM64 | [`jj-git-prompt-linux-arm64`](https://github.com/rselbach/jj-git-prompt/releases/latest/download/jj-git-prompt-linux-arm64) |
| macOS x64 (`amd64`) | [`jj-git-prompt-darwin-amd64`](https://github.com/rselbach/jj-git-prompt/releases/latest/download/jj-git-prompt-darwin-amd64) |
| macOS ARM64 | [`jj-git-prompt-darwin-arm64`](https://github.com/rselbach/jj-git-prompt/releases/latest/download/jj-git-prompt-darwin-arm64) |

After downloading a binary, mark it executable with `chmod +x` and move it to
a directory in `PATH`.

Or build the repository from source:

```sh
go build -o jj-git-prompt ./cmd/jj-git-prompt
```

The default command prints the prompt segment for the current repository:

```sh
jj-git-prompt
jj-git-prompt --no-color
jj-git-prompt --cwd /path/to/repository
```

## Starship

Add a custom module to `~/.config/starship.toml`:

```toml
[custom.jj]
when = "jj-git-prompt detect"
shell = ["jj-git-prompt"]
format = "$output "
```

Disable Starship's built-in Git modules if this module should replace them:

```toml
[git_branch]
disabled = true

[git_status]
disabled = true
```

For a styled Starship module that supplies its own colors, invoke the command
without ANSI styling or repository prefixes:

```toml
[custom.jj]
when = "jj-git-prompt detect"
shell = ["jj-git-prompt", "--no-color", "--no-symbol", "--no-jj-prefix", "--no-git-prefix"]
format = "$output "
```

## Commands

Usage: `jj-git-prompt [prompt|detect|version] [flags]`

| Command | Behavior |
| --- | --- |
| `prompt` | Print the prompt segment. This is the default when no command is given. |
| `detect` | Exit successfully when the current directory is inside a JJ or Git repository; otherwise exit with status 1. |
| `version` | Print the version and enabled features. |

Flags may appear before or after the command.

## CLI Options

| Option | Description |
| --- | --- |
| `--cwd PATH` | Detect and inspect the repository containing `PATH` instead of the current directory. |
| `--bash` | Wrap ANSI escape sequences as non-printing characters for Bash Readline. |
| `--no-color` | Disable all ANSI styling. |
| `--truncate-name N` | Limit branch and bookmark names to `N` characters, including the ellipsis; `0` is unlimited (default: `0`). |
| `--id-length N` | Display `N` characters of the JJ change ID or Git commit hash (default: `8`). |
| `--ancestor-bookmark-depth N` | Search at most `N` ancestors for JJ bookmarks; `0` disables the search (default: `10`). |
| `--bookmarks-display-limit N` | Display at most `N` JJ bookmarks; `0` is unlimited (default: `3`). |
| `--strip-bookmark-prefix PREFIXES` | Strip the first matching prefix from JJ bookmark names. Accepts a comma-separated list. |
| `--jj-symbol SYMBOL` | Set the JJ repository symbol (default: `󱗆 `). |
| `--git-symbol SYMBOL` | Set the Git repository symbol (default: ` `). |
| `--no-symbol` | Hide the JJ or Git symbol while retaining any enabled `on ` prefix. |
| `--no-jj-prefix` | Hide the JJ `on {symbol}` prefix. |
| `--no-jj-name` | Hide JJ bookmark names. |
| `--no-jj-id` | Hide the JJ change ID. |
| `--no-jj-status` | Hide JJ status symbols. |
| `--no-prefix-color` | Disable JJ change-ID unique-prefix highlighting. |
| `--no-git-prefix` | Hide the Git `on {symbol}` prefix. |
| `--no-git-name` | Hide the Git branch name. Detached HEAD is displayed as `HEAD` when enabled. |
| `--no-git-id` | Hide the Git commit hash. |
| `--no-git-status` | Hide Git status symbols. |
| `-h`, `--help` | Print usage and all CLI options. |

## Environment Variables

CLI values take precedence over their environment-variable equivalents.

| Variable | Equivalent option | Default |
| --- | --- | --- |
| `JJ_GIT_PROMPT_TRUNCATE_NAME` | `--truncate-name` | `0` |
| `JJ_GIT_PROMPT_ID_LENGTH` | `--id-length` | `8` |
| `JJ_GIT_PROMPT_ANCESTOR_BOOKMARK_DEPTH` | `--ancestor-bookmark-depth` | `10` |
| `JJ_GIT_PROMPT_BOOKMARKS_DISPLAY_LIMIT` | `--bookmarks-display-limit` | `3` |
| `JJ_GIT_PROMPT_STRIP_BOOKMARK_PREFIX` | `--strip-bookmark-prefix` | empty |
| `JJ_GIT_PROMPT_JJ_SYMBOL` | `--jj-symbol` | `󱗆 ` |
| `JJ_GIT_PROMPT_GIT_SYMBOL` | `--git-symbol` | ` ` |

The following presence-based variables disable individual display elements.
Their values are ignored, so setting one to `0` or `false` still disables the
element.

| Variable | Equivalent option or behavior |
| --- | --- |
| `JJ_GIT_PROMPT_NO_PREFIX_COLOR` | `--no-prefix-color` |
| `JJ_GIT_PROMPT_NO_JJ_PREFIX` | `--no-jj-prefix` |
| `JJ_GIT_PROMPT_NO_JJ_NAME` | `--no-jj-name` |
| `JJ_GIT_PROMPT_NO_JJ_ID` | `--no-jj-id` |
| `JJ_GIT_PROMPT_NO_JJ_STATUS` | `--no-jj-status` |
| `JJ_GIT_PROMPT_NO_JJ_COLOR` | Disable ANSI styling for JJ output. |
| `JJ_GIT_PROMPT_NO_GIT_PREFIX` | `--no-git-prefix` |
| `JJ_GIT_PROMPT_NO_GIT_NAME` | `--no-git-name` |
| `JJ_GIT_PROMPT_NO_GIT_ID` | `--no-git-id` |
| `JJ_GIT_PROMPT_NO_GIT_STATUS` | `--no-git-status` |
| `JJ_GIT_PROMPT_NO_GIT_COLOR` | Disable ANSI styling for Git output. |

`--cwd`, `--bash`, `--no-color`, and `--no-symbol` do not have environment
variable equivalents.

## Output

JJ repositories use this format:

```text
on {symbol}{change_id} ({bookmarks}) [{status}]
```

Ancestor bookmarks include their distance, such as `main~3`. When the display
limit hides bookmarks, an `…+N` marker reports how many were omitted.

| JJ status | Meaning |
| --- | --- |
| `!` | The working-copy commit has conflicts. |
| `⇔` | The change ID is divergent. |
| `∅` | A non-empty working-copy commit has no description. |
| `⇡` | The current or closest bookmark has a remote counterpart but is not synchronized. |

Git repositories use this format:

```text
on {symbol}{branch} ({commit}) [{status}]
```

| Git status | Meaning |
| --- | --- |
| `=` | Conflicted files. |
| `+` | Staged changes. |
| `!` | Modified files. |
| `?` | Untracked files. |
| `✘` | Deleted files. |
| `⇡N` | `N` commits ahead of the upstream branch. |
| `⇣N` | `N` commits behind the upstream branch. |

When a repository contains both `.jj` and `.git`, JJ output takes precedence.
