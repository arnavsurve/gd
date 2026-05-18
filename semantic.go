package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

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
	Summary semSummary  `json:"summary"`
	Changes []semChange `json:"changes"`
}

var (
	semChecked   bool
	semInstalled bool
	semMu        sync.Mutex
)

func semAvailable() bool {
	semMu.Lock()
	defer semMu.Unlock()
	if !semChecked {
		_, err := exec.LookPath("sem")
		semInstalled = err == nil
		semChecked = true
	}
	return semInstalled
}

func runSemDiff() (*semOutput, error) {
	args := []string{"diff", "--format", "json"}

	if commitFrom != "" {
		args = append(args, "--from", commitFrom, "--to", commitTo)
	} else if flagBase != "" {
		args = append(args, "--from", flagBase, "--to", "HEAD")
	}

	out, err := exec.Command("sem", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("sem diff: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	var result semOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, nil
	}
	return &result, nil
}

func runSemDiffStaged() (*semOutput, error) {
	args := []string{"diff", "--staged", "--format", "json"}
	out, err := exec.Command("sem", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("sem diff --staged: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	var result semOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, nil
	}
	return &result, nil
}

type semCache struct {
	unstaged *semOutput
	staged   *semOutput
}

func (c *semCache) changesForFile(path string, f fileStatus) []semChange {
	if c == nil {
		return nil
	}
	var changes []semChange
	if f.staged && c.staged != nil {
		for _, ch := range c.staged.Changes {
			if ch.FilePath == path {
				changes = append(changes, ch)
			}
		}
	}
	if (f.unstaged || f.untracked) && c.unstaged != nil {
		for _, ch := range c.unstaged.Changes {
			if ch.FilePath == path {
				changes = append(changes, ch)
			}
		}
	}
	if commitFrom != "" || flagBase != "" {
		if c.unstaged != nil {
			for _, ch := range c.unstaged.Changes {
				if ch.FilePath == path {
					changes = append(changes, ch)
				}
			}
		}
	}
	return changes
}

func (c *semCache) summaryForFile(path string, f fileStatus) semSummary {
	changes := c.changesForFile(path, f)
	var s semSummary
	for _, ch := range changes {
		switch ch.ChangeType {
		case "added":
			s.Added++
		case "modified":
			s.Modified++
		case "deleted":
			s.Deleted++
		case "renamed":
			s.Renamed++
		case "moved":
			s.Moved++
		}
		s.Total++
	}
	return s
}

func renderSemSummary(s semSummary, width int) string {
	var parts []string
	if s.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", s.Modified))
	}
	if s.Renamed > 0 {
		parts = append(parts, fmt.Sprintf("%d renamed", s.Renamed))
	}
	if s.Moved > 0 {
		parts = append(parts, fmt.Sprintf("%d moved", s.Moved))
	}
	if s.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", s.Added))
	}
	if s.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", s.Deleted))
	}
	if len(parts) == 0 {
		return ""
	}
	text := " " + strings.Join(parts, " · ")
	return summarySty.Render(fitStr(text, width))
}

func semChangeHeader(ch semChange, width int) string {
	kind := ch.EntityType
	name := ch.EntityName

	var label string
	var sty lipgloss.Style

	switch ch.ChangeType {
	case "modified":
		label = "(modified)"
		sty = hunkHdrSty
	case "added":
		label = "(added)"
		sty = addIndSty
	case "deleted":
		label = "(deleted)"
		sty = delIndSty
	case "renamed":
		if ch.BeforeContent != nil {
			oldName := extractEntityName(ch)
			if oldName != "" && oldName != name {
				name = oldName + " → " + name
			}
		}
		label = "(renamed)"
		sty = renamedHdrSty
	case "moved":
		dest := ch.FilePath
		if ch.OldFilePath != nil {
			dest = *ch.OldFilePath + " → " + ch.FilePath
		}
		label = "(moved: " + dest + ")"
		sty = movedHdrSty
	default:
		label = "(" + ch.ChangeType + ")"
		sty = hunkHdrSty
	}

	prefix := "── " + kind + " " + name + " "
	text := prefix + label + " "
	pad := width - len([]rune(text))
	if pad > 0 {
		text += strings.Repeat("─", pad)
	}
	return sty.Render(text)
}

func extractEntityName(ch semChange) string {
	parts := strings.Split(ch.EntityID, "::")
	if len(parts) >= 3 {
		oldParts := strings.SplitN(ch.EntityID, "::", 3)
		if ch.BeforeContent != nil {
			lines := strings.SplitN(*ch.BeforeContent, "\n", 2)
			if len(lines) > 0 {
				first := lines[0]
				first = strings.TrimPrefix(first, "func ")
				if idx := strings.IndexByte(first, '('); idx > 0 {
					return first[:idx]
				}
				if idx := strings.IndexByte(first, ' '); idx > 0 {
					return first[:idx]
				}
			}
			_ = oldParts
		}
	}
	return ""
}

func renderSemanticDiff(cache *semCache, f fileStatus, width int) (string, []int) {
	if width <= 0 {
		width = 80
	}

	changes := cache.changesForFile(f.path, f)
	if len(changes) == 0 {
		return "", nil
	}

	summary := cache.summaryForFile(f.path, f)
	hl := newHighlighter(f.path)

	var b strings.Builder
	var hunkLines []int
	lineCount := 0

	sumText := renderSemSummary(summary, width)
	if sumText != "" {
		b.WriteString(sumText)
		b.WriteByte('\n')
		lineCount++
		b.WriteByte('\n')
		lineCount++
	}

	for i, ch := range changes {
		if i > 0 {
			b.WriteByte('\n')
			lineCount++
		}

		hunkLines = append(hunkLines, lineCount)
		b.WriteString(semChangeHeader(ch, width))
		b.WriteByte('\n')
		lineCount++

		before := b.Len()

		switch ch.ChangeType {
		case "modified", "renamed":
			renderSemModified(&b, ch, width, hl)
		case "added":
			renderSemAdded(&b, ch, width, hl)
		case "deleted":
			renderSemDeleted(&b, ch, width, hl)
		case "moved":
			renderSemMoved(&b, ch, width, hl)
		}

		lineCount += strings.Count(b.String()[before:], "\n")
	}

	return b.String(), hunkLines
}

func renderSemModified(b *strings.Builder, ch semChange, width int, hl *highlighter) {
	if ch.BeforeContent == nil || ch.AfterContent == nil {
		return
	}
	oldLines := strings.Split(*ch.BeforeContent, "\n")
	newLines := strings.Split(*ch.AfterContent, "\n")

	diff := diffLines(oldLines, newLines)

	if width >= sideBySideMinWidth {
		renderSemSideBySide(b, diff, width, hl)
	} else {
		renderSemUnified(b, diff, width, hl)
	}
}

func renderSemAdded(b *strings.Builder, ch semChange, width int, hl *highlighter) {
	if ch.AfterContent == nil {
		return
	}
	const numW = 4
	textW := width - numW - 2
	if textW < 10 {
		textW = 10
	}
	lines := strings.Split(*ch.AfterContent, "\n")
	for i, line := range lines {
		chunks := wrapText(line, textW)
		for ci, chunk := range chunks {
			if ci == 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, i+1)))
				b.WriteString("  ")
			} else {
				b.WriteString(strings.Repeat(" ", numW+2))
			}
			b.WriteString(hl.renderLine(chunk, textW, bgAdd))
			b.WriteByte('\n')
		}
	}
}

func renderSemDeleted(b *strings.Builder, ch semChange, width int, hl *highlighter) {
	if ch.BeforeContent == nil {
		return
	}
	const numW = 4
	textW := width - numW - 2
	if textW < 10 {
		textW = 10
	}
	lines := strings.Split(*ch.BeforeContent, "\n")
	for i, line := range lines {
		chunks := wrapText(line, textW)
		for ci, chunk := range chunks {
			if ci == 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, i+1)))
				b.WriteString("  ")
			} else {
				b.WriteString(strings.Repeat(" ", numW+2))
			}
			b.WriteString(hl.renderLine(chunk, textW, bgDel))
			b.WriteByte('\n')
		}
	}
}

func renderSemMoved(b *strings.Builder, ch semChange, width int, hl *highlighter) {
	content := ch.AfterContent
	if content == nil {
		content = ch.BeforeContent
	}
	if content == nil {
		return
	}
	const numW = 4
	textW := width - numW - 2
	if textW < 10 {
		textW = 10
	}
	lines := strings.Split(*content, "\n")
	for i, line := range lines {
		chunks := wrapText(line, textW)
		for ci, chunk := range chunks {
			if ci == 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, i+1)))
				b.WriteString("  ")
			} else {
				b.WriteString(strings.Repeat(" ", numW+2))
			}
			b.WriteString(hl.renderLine(chunk, textW, bgMov))
			b.WriteByte('\n')
		}
	}
}

type diffOp int

const (
	diffEqual diffOp = iota
	diffInsert
	diffDelete
)

type diffLine struct {
	op   diffOp
	text string
}

func diffLines(old, new []string) []diffLine {
	m, n := len(old), len(new)

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if old[i-1] == new[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var result []diffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && old[i-1] == new[j-1] {
			result = append(result, diffLine{diffEqual, old[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			result = append(result, diffLine{diffInsert, new[j-1]})
			j--
		} else {
			result = append(result, diffLine{diffDelete, old[i-1]})
			i--
		}
	}

	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

type diffGroup struct {
	op    diffOp
	lines []string
}

func groupDiffLines(lines []diffLine) []diffGroup {
	var groups []diffGroup
	for _, l := range lines {
		if len(groups) > 0 && groups[len(groups)-1].op == l.op {
			groups[len(groups)-1].lines = append(groups[len(groups)-1].lines, l.text)
		} else {
			groups = append(groups, diffGroup{op: l.op, lines: []string{l.text}})
		}
	}
	return groups
}

func renderSemSideBySide(b *strings.Builder, lines []diffLine, width int, hl *highlighter) {
	const numW = 4
	colW := (width - numW*2 - 5) / 2
	if colW < 10 {
		colW = 10
	}

	groups := groupDiffLines(lines)
	oldNum := 1
	newNum := 1

	emitRow := func(lNum int, lText string, lBg diffBg, rNum int, rText string, rBg diffBg) {
		lChunks := wrapText(lText, colW)
		rChunks := wrapText(rText, colW)
		maxRows := len(lChunks)
		if len(rChunks) > maxRows {
			maxRows = len(rChunks)
		}
		for row := 0; row < maxRows; row++ {
			if row == 0 && lNum > 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, lNum)))
			} else {
				b.WriteString(strings.Repeat(" ", numW))
			}
			b.WriteByte(' ')
			if row < len(lChunks) {
				b.WriteString(hl.renderLine(lChunks[row], colW, lBg))
			} else {
				b.WriteString(hl.renderLine("", colW, lBg))
			}
			b.WriteString(gutterSty.Render(" │ "))
			if row == 0 && rNum > 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, rNum)))
			} else {
				b.WriteString(strings.Repeat(" ", numW))
			}
			b.WriteByte(' ')
			if row < len(rChunks) {
				b.WriteString(hl.renderLine(rChunks[row], colW, rBg))
			} else {
				b.WriteString(hl.renderLine("", colW, rBg))
			}
			b.WriteByte('\n')
		}
	}

	for i := 0; i < len(groups); i++ {
		g := groups[i]
		switch g.op {
		case diffEqual:
			for _, text := range g.lines {
				emitRow(oldNum, text, bgNone, newNum, text, bgNone)
				oldNum++
				newNum++
			}
		case diffDelete:
			var addGrp *diffGroup
			if i+1 < len(groups) && groups[i+1].op == diffInsert {
				addGrp = &groups[i+1]
				i++
			}
			maxLen := len(g.lines)
			if addGrp != nil && len(addGrp.lines) > maxLen {
				maxLen = len(addGrp.lines)
			}
			for j := 0; j < maxLen; j++ {
				var lNum int
				var lText string
				lBg := bgDel
				var rNum int
				var rText string
				rBg := bgAdd
				if j < len(g.lines) {
					lNum = oldNum
					lText = g.lines[j]
					oldNum++
				} else {
					lBg = bgNone
				}
				if addGrp != nil && j < len(addGrp.lines) {
					rNum = newNum
					rText = addGrp.lines[j]
					newNum++
				} else {
					rBg = bgNone
				}
				emitRow(lNum, lText, lBg, rNum, rText, rBg)
			}
		case diffInsert:
			for _, text := range g.lines {
				emitRow(0, "", bgNone, newNum, text, bgAdd)
				newNum++
			}
		}
	}
}

func renderSemUnified(b *strings.Builder, lines []diffLine, width int, hl *highlighter) {
	const numW = 4
	textW := width - numW*2 - 4
	if textW < 10 {
		textW = 10
	}

	oldNum := 1
	newNum := 1
	blankPrefix := strings.Repeat(" ", numW*2+4)

	for _, line := range lines {
		chunks := wrapText(line.text, textW)
		for ci, chunk := range chunks {
			switch line.op {
			case diffEqual:
				if ci == 0 {
					b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*d", numW, oldNum, numW, newNum)))
					b.WriteString("   ")
				} else {
					b.WriteString(blankPrefix)
				}
				b.WriteString(hl.renderLine(chunk, textW, bgNone))
			case diffDelete:
				if ci == 0 {
					b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*s", numW, oldNum, numW, "")))
					b.WriteString(delIndSty.Render(" -"))
					b.WriteByte(' ')
				} else {
					b.WriteString(blankPrefix)
				}
				b.WriteString(hl.renderLine(chunk, textW, bgDel))
			case diffInsert:
				if ci == 0 {
					b.WriteString(lineNumSty.Render(fmt.Sprintf("%*s %*d", numW, "", numW, newNum)))
					b.WriteString(addIndSty.Render(" +"))
					b.WriteByte(' ')
				} else {
					b.WriteString(blankPrefix)
				}
				b.WriteString(hl.renderLine(chunk, textW, bgAdd))
			}
			b.WriteByte('\n')
		}
		switch line.op {
		case diffEqual:
			oldNum++
			newNum++
		case diffDelete:
			oldNum++
		case diffInsert:
			newNum++
		}
	}
}
