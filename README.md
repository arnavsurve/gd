# gd

A terminal git diff viewer with tree-based file browsing.

## Install

### From release binaries

Download the latest binary from [Releases](https://github.com/arnavsurve/gd/releases) and place it on your PATH.

### From source

Requires Go 1.21+.

```
go install github.com/arnavsurve/gd@latest
```

If you have the oh-my-zsh git plugin, you'll need to unalias `gd` in your `.zshrc`:

```
unalias gd 2>/dev/null
```

## Usage

Run `gd` in a git repo.

```
gd                    # browse staged, unstaged, and untracked files
gd --main             # browse files changed vs main branch
gd --base feature/x   # browse files changed vs an arbitrary base branch
gd @1                 # diff of last commit
gd @3                 # diff of last 3 commits
gd @2..4              # diff of commits 2 through 4 back
```

### Navigation

| Key | Action |
|-----|--------|
| `j` / `k` or `↑` / `↓` | navigate file tree |
| `enter` | open fullscreen diff view |
| `/` | search / filter files |
| `esc` | clear search, or quit |
| `q` | quit |

### Diff Controls

| Key | Action |
|-----|--------|
| `f` | toggle full file diff vs hunk context |
| `+` / `-` | expand / shrink context around hunks (±4 lines) |
| `e` | open file in `$EDITOR` |

### Fullscreen Diff

| Key | Action |
|-----|--------|
| `n` / `p` | jump to next / previous hunk |
| `f` | toggle full file diff vs hunk context |
| `+` / `-` | expand / shrink context around hunks (±4 lines) |
| `e` | open file in `$EDITOR` |
| `q` / `esc` | back to split view |

### Sidebar

The sidebar is resizable — drag the border between the file tree and diff pane to adjust. The ratio persists across sessions.

Mouse scroll works in both the file tree and the diff pane. Click a file in the tree to select it.

### Pager Mode

`gd` can be used as a diff pager by piping diff output to it. It reads from stdin, renders with syntax highlighting and side-by-side formatting, and prints to stdout.

```
git diff | gd
git show HEAD | gd
git log -p | gd
```

#### As git's default pager

```
git config --global pager.diff gd
git config --global pager.show gd
git config --global pager.log gd
```

#### With lazygit

```yaml
# ~/.config/lazygit/config.yml (Linux)
# ~/Library/Application Support/lazygit/config.yml (macOS)
git:
  paging:
    colorArg: never
    pager: gd
```

Setting `colorArg: never` tells git not to add its own colors so `gd` can handle all the rendering.

### Themes

| Key | Action |
|-----|--------|
| `t` | toggle dark / light mode |
| `T` | open syntax theme picker |

`gd` detects your system theme at startup. Press `t` to toggle manually (useful in tmux where system theme detection can be unreliable).

The theme picker shows only dark themes in dark mode and light themes in light mode, with a live preview as you scroll. Press `enter` to confirm or `esc` to cancel.

Theme selections are saved per mode to `~/.config/gd/config.json` (or `~/Library/Application Support/gd/config.json` on macOS) and persist across sessions.
