# gd

A terminal git diff viewer. Browse changed files in a tree, see diffs with syntax highlighting, and open full-file views in less.

## Install

Requires Go 1.21+.

```
git clone https://github.com/arnavsurve/gd.git
cd gd
go build -o gd .
```

Then either move the binary somewhere on your PATH:

```
mv gd /usr/local/bin/
```

Or alias it in your shell config:

```
alias gd="$HOME/dev/gd/gd"
```

If you have the oh-my-zsh git plugin, you'll need to unalias `gd` first:

```
unalias gd 2>/dev/null
alias gd="$HOME/dev/gd/gd"
```

## Usage

Run `gd` in a git repo.

```
gd          # browse staged, unstaged, and untracked files
gd --main   # browse files changed vs main branch
```

### Controls

| Key | Action |
|-----|--------|
| `j` / `k` or arrow keys | navigate file tree |
| `enter` | open full-file diff in less |
| `q` in less | back to file browser |
| `/` | search files |
| `t` | toggle dark / light mode |
| `T` | open syntax theme picker |
| `esc` | clear search, or quit |
| `q` | quit |

### Themes

`gd` detects your system theme at startup. Press `t` to toggle between dark and light mode manually (useful in tmux where system theme changes aren't always detected).

Press `T` to open the theme picker. It shows only dark themes in dark mode and light themes in light mode, with a live preview as you scroll. Press `enter` to confirm or `esc` to cancel.

Theme selections are saved per mode to `~/.config/gd/config.json` (or `~/Library/Application Support/gd/config.json` on macOS) and persist across sessions.
