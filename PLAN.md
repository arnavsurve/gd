# Semantic Diffs in gd

## Overview

Add semantic diffing to gd by shelling out to [`sem`](https://github.com/ataraxy-labs/sem) for entity-level analysis and rendering the results in gd's TUI. `sem` handles all the hard parts (tree-sitter parsing, structural hashing, three-tier matching, rename/move detection). `gd` consumes `sem diff --format json` and focuses on what it's good at: rendering a beautiful, navigable diff.

No new parsing dependencies. No tree-sitter. No CGo. gd stays a pure Go binary.

## Architecture

```
sem diff --format json
        │
        ▼
   JSON output
   (entities with before/after content,
    change types, structural hashes)
        │
        ▼
   gd parses JSON, renders each entity
   with structural headers, syntax highlighting,
   side-by-side layout, and navigation
```

## sem JSON Format

`sem diff --format json` returns:

```json
{
  "summary": {
    "fileCount": 2,
    "added": 1,
    "modified": 2,
    "deleted": 1,
    "renamed": 1,
    "moved": 0,
    "total": 5
  },
  "changes": [
    {
      "afterContent": "func expandTabs(s string) string {\n\treturn strings.ReplaceAll(s, \"\\t\", \"        \")\n}",
      "author": null,
      "beforeContent": "func expandTabs(s string) string {\n\treturn strings.ReplaceAll(s, \"\\t\", \"    \")\n}",
      "changeType": "modified",
      "commitSha": null,
      "entityId": "diff.go::function::expandTabs",
      "entityName": "expandTabs",
      "entityType": "function",
      "filePath": "diff.go",
      "oldFilePath": null
    },
    {
      "afterContent": "func stripLineEndings(s string) string { ... }",
      "author": null,
      "beforeContent": "func trimLine(s string) string { ... }",
      "changeType": "renamed",
      "commitSha": null,
      "entityId": "diff.go::function::stripLineEndings",
      "entityName": "stripLineEndings",
      "entityType": "function",
      "filePath": "diff.go",
      "oldFilePath": null
    }
  ]
}
```

Change types: `added`, `modified`, `deleted`, `renamed`, `moved`.

Each change has flat fields — no nesting for name/body pairs. `beforeContent` is null for added entities, `afterContent` is null for deleted entities. `oldFilePath` is non-null for moved entities.

### sem CLI flags mapping to gd modes

| gd mode | sem invocation |
|---|---|
| `gd` (unstaged) | `sem diff --format json` |
| `gd` (staged) | `sem diff --staged --format json` |
| `gd --main` | `sem diff --from main --to HEAD --format json` |
| `gd @1` | `sem diff --commit HEAD --format json` |
| `gd @N` | `sem diff --from HEAD~N --to HEAD --format json` |
| `gd @2..4` | `sem diff --from HEAD~4 --to HEAD~2 --format json` |

### Limitations discovered during testing

- **No per-file filtering**: `sem diff` operates on the whole repo. We run it once, cache the output, and index by `filePath`.
- **Empty output on no changes**: `sem diff --staged` with nothing staged prints a terminal message but no JSON. Must handle non-JSON / empty output gracefully.
- **Untracked files**: `sem diff` does detect untracked files (tested with PLAN.md).

## UX Design

### Toggle

- `s` key toggles semantic mode on/off in both split and fullscreen views
- Config persistence: `"semantic": true/false` in `~/.config/gd/config.json`
- CLI flag: `--semantic` / `--no-semantic` to override per invocation
- Default: off (opt-in)
- Status bar shows `semantic` indicator when active
- Requires `sem` to be on `$PATH`. If missing, `s` key does nothing and `--semantic` prints a message pointing to the install instructions.

### Hunk Headers

Current:
```
@@ -12,8 +14,10 @@ func main()
```

Semantic mode:
```
── func ProcessOrder  (modified) ──────────────────
── func ValidateInput (moved → validators.go) ─────
── func oldName → newName (renamed) ────────────────
── func NewHelper     (added) ─────────────────────
── func OldHelper     (deleted) ───────────────────
```

### Change Summary

One-line summary rendered above the diff content:

```
 2 modified · 1 renamed · 1 added · 1 deleted
```

### Rendering by Status

- **modified**: Side-by-side (or unified) diff of `beforeContent` vs `afterContent`, using gd's existing diff rendering. Generate a unified diff from the two bodies, pass through `renderDiff()`.
- **renamed**: Header shows `oldName → newName`. If bodies are identical (same structural hash), show dimmed single-column view of the body. If bodies also differ, show a diff like modified.
- **moved**: Header shows destination path. Body rendered dimmed as context (no red/green) since the code itself didn't change.
- **added**: All lines rendered with add background.
- **deleted**: All lines rendered with delete background.

### Collapsed Unchanged

Between changed entities, collapsed unchanged regions:

```
  ··· func Foo, func Bar (unchanged) ···
```

### Color

- Green/red backgrounds: same as current for add/delete lines within modified/renamed entities
- Blue/cyan tint on hunk header: moved entities
- Yellow/amber tint on hunk header: renamed entities
- Standard hunk header color: modified/added/deleted entities

### Fallback

- If `sem` is not installed: semantic mode unavailable, `s` does nothing
- If `sem` returns an error or empty output for a file (unsupported language, parse failure): fall back to line-based diff for that file silently
- Pager mode: semantic mode available if `sem` is on PATH and we're in a git repo. `sem` handles its own git plumbing.

## Implementation Plan

### Phase 1: sem integration

**New file: `semantic.go`**

Types to deserialize sem's JSON:

```go
type semSummary struct {
    FileCount int `json:"fileCount"`
    Added     int `json:"added"`
    Modified  int `json:"modified"`
    Deleted   int `json:"deleted"`
    Renamed   int `json:"renamed"`
    Moved     int `json:"moved"`
    Total     int `json:"total"`
}

type semChange struct {
    AfterContent  *string `json:"afterContent"`
    Author        *string `json:"author"`
    BeforeContent *string `json:"beforeContent"`
    ChangeType    string  `json:"changeType"`
    CommitSha     *string `json:"commitSha"`
    EntityID      string  `json:"entityId"`
    EntityName    string  `json:"entityName"`
    EntityType    string  `json:"entityType"`
    FilePath      string  `json:"filePath"`
    OldFilePath   *string `json:"oldFilePath"`
}

type semOutput struct {
    Summary semSummary `json:"summary"`
    Changes []semChange `json:"changes"`
}
```

Functions:

- `semAvailable() bool` — checks if `sem` is on `$PATH` via `exec.LookPath`. Result cached in a package-level var.
- `runSemDiff() (*semOutput, error)` — shells out to `sem diff --format json` (with appropriate flags for the current mode: `--staged`, `--from`/`--to`, `--commit`). Returns the full repo output. Handles empty/non-JSON output by returning nil.
- `semChangesForFile(output *semOutput, path string) []semChange` — filters the cached output to changes matching a specific `filePath`.

**Caching in model:**

Since `sem` has no per-file filtering, we run it once and cache. The model gets:
```go
type model struct {
    // ...existing fields...
    semantic    bool
    semCache    *semOutput  // cached sem output, nil if not loaded
    semLoading  bool        // true while sem is running
}
```

Cache is invalidated on file refresh (tick) or when toggling semantic mode. Loading is async (tea.Cmd) to avoid blocking the UI.

### Phase 2: Rendering semantic changes

**In `semantic.go`:**

- `renderSemanticDiff(sem *semOutput, width int, filename string) (string, []int)` — produces rendered string and hunk line positions for navigation. For each change:
  - Render structural header with entity type, name, and status badge
  - For `modified`: generate unified diff from `beforeContent` / `afterContent` using Go's `diff` package or line-level diffing, then render through existing `renderSideBySide` / `renderUnified`
  - For `renamed`: header shows `old → new`. If same structural hash, render body dimmed. If hash differs, render as modified.
  - For `moved`: header with destination, body dimmed as context
  - For `added`: render `afterContent` with add background
  - For `deleted`: render `beforeContent` with delete background
  - Between changes, render collapsed unchanged: `··· func X, func Y (unchanged) ···`

- `renderSemSummary(sem *semOutput, width int) string` — the one-line summary

- `diffBodies(before, after string) string` — generates a unified diff string from two raw bodies. Can use `github.com/sergi/go-diff/diffmatchpatch` or a simpler line-differ to produce gitdiff-compatible output that feeds into the existing `renderDiff` pipeline.

For the body diffing, we need to turn two raw strings into something renderable by the existing `renderSideBySide`/`renderUnified` code. Two options:
1. Generate a synthetic unified diff (`--- a/...\n+++ b/...\n@@...`) and feed it through `renderDiff()` — reuses all existing rendering.
2. Do our own line-level diff and render directly — more control but duplicates rendering logic.

Option 1 is strongly preferred. `go-diff` or even a simple LCS line differ can produce the unified format.

### Phase 3: Model and config integration

**`config.go` changes:**

Add to config struct:
```go
Semantic *bool `json:"semantic,omitempty"`
```

Use `*bool` to distinguish unset (nil → default off) from explicit true/false.

**`main.go` changes:**

- Add `--semantic` / `--no-semantic` flags
- Flag overrides config, config overrides default (off)
- On `--semantic` without `sem` installed: print error with install URL, exit

**`model.go` changes:**

- Add `semantic bool`, `semCache *semOutput`, `semLoading bool` fields
- `loadPreview()` and `openFullDiff()` branch on `m.semantic`:
  - If true and `semCache` has data for the selected file: call `renderSemanticDiff()` with the filtered changes
  - If true but no cache yet: trigger async `runSemDiff()`, show "Loading semantic diff..." placeholder
  - If false or error: fall back to current `getDiffOutput()` + `renderDiff()`

**`update.go` changes:**

- Handle `s` key in normal and fullscreen modes:
  - If `sem` not available: no-op
  - Otherwise: toggle `m.semantic`. If turning on and no cache, trigger async sem load.
- New message types:
  ```go
  type semLoadedMsg struct {
      output *semOutput
      err    error
  }

  type semanticDiffLoadedMsg struct {
      content   string
      hunkLines []int
  }
  ```
- On `semLoadedMsg`: store in `m.semCache`, set `m.semLoading = false`, trigger preview reload
- On `filesRefreshedMsg` (tick): if `m.semantic`, invalidate `m.semCache` and re-run sem
- On `tickMsg`: only re-run sem if files actually changed (compare with previous file list, same as current behavior)

**`view.go` changes:**

- Status bar: append `semantic` when `m.semantic` is true
- Status bar hint: add `s sem` to the keybinding help

### Phase 4: Theme support

**`theme.go` changes:**

Add to palette:
```go
bgMoved   string // subtle blue tint for moved blocks
movedHdr  string // moved header color (cyan/blue)
renamedHdr string // renamed header color (yellow/amber)
summaryFg string // change summary text color
```

Dark values: `bgMoved: "#1a1f2b"`, `movedHdr: "#58a6ff"`, `renamedHdr: "#d29922"`, `summaryFg: "#8b949e"`
Light values: `bgMoved: "#ddf4ff"`, `movedHdr: "#0969da"`, `renamedHdr: "#9a6700"`, `summaryFg: "#656d76"`

Add `bgMoved` to the `diffBg` enum and `bgColors` map.

Add corresponding lipgloss styles: `movedHdrSty`, `renamedHdrSty`, `summarySty`.

### Phase 5: Pager mode (deferred to v2)

Pager mode semantic support is deferred. The complication: piped diff text may come from any ref/context, and sem operates on the actual repo state. Running sem separately in pager mode could produce different results than the piped diff. Need a clean solution before shipping this.

For v1, pager mode continues to use line-based rendering regardless of the semantic config setting.

## File Summary

| File | Change |
|---|---|
| `semantic.go` | **New.** sem types, invocation, rendering, body diffing |
| `config.go` | Add `Semantic` field |
| `main.go` | Add `--semantic`/`--no-semantic` flag, pager mode integration |
| `model.go` | Add `semantic` field, branch preview/diff loading |
| `update.go` | Handle `s` key, new message type |
| `theme.go` | Add moved/renamed/summary colors and styles |
| `view.go` | Show `semantic` in status bar |
| `go.mod` | Possibly add a line-diffing library (e.g. `go-diff`) for body comparison |

## Dependencies

- **Runtime**: `sem` CLI on `$PATH` (not a build dependency — graceful degradation if missing)
- **Build**: possibly `github.com/sergi/go-diff` for diffing `beforeContent` vs `afterContent` strings into unified format. Alternatively, write a simple line differ — the bodies are usually small.

## Open Questions (resolved during testing)

- ✅ **sem CLI flags**: map cleanly to gd modes. See table above.
- ✅ **Per-file filtering**: sem is whole-repo only. Solution: run once, cache, filter by `filePath`.
- ✅ **JSON shape**: flat fields, not nested. `beforeContent`/`afterContent` are nullable strings. Schema documented above.
- ✅ **Empty output**: sem prints a terminal message with no JSON when there are no changes. Handle by checking for valid JSON.
- ✅ **Untracked files**: sem does detect them.

## Remaining Open Questions

- **Collapsed unchanged entities**: sem only returns *changed* entities. To show "··· func Foo, func Bar (unchanged) ···" between changes, we'd need to know what unchanged entities exist. This would require parsing the file separately or running a second sem command. **Defer to v2** — in v1 just show the changed entities sequentially with no collapsed unchanged markers.
- **Performance on large repos**: sem was fast on the gd repo (~1800 LOC). Need to test on larger repos. The caching approach mitigates this — we only re-run on file changes.
- **Mixed staged+unstaged**: gd shows both staged and unstaged changes together. sem has `--staged` for staged-only and default for unstaged-only. We may need to run sem twice and merge results, or just run without `--staged` (which seems to show all working tree changes including staged).
- **Pager mode**: sem operates on a git repo, not on piped diff text. In pager mode, if we're in a git repo, we could potentially run sem separately. But the piped diff might be from a different ref than what sem would compute. **Defer pager mode semantic support to v2.**
