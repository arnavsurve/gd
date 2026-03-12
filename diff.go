package main

import (
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const sideBySideMinWidth = 120

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func wrapText(text string, w int) []string {
	if w <= 0 {
		return []string{text}
	}
	text = expandTabs(text)
	runes := []rune(text)
	if len(runes) <= w {
		return []string{string(runes)}
	}
	var chunks []string
	for len(runes) > 0 {
		end := w
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

func trimLine(s string) string {
	return strings.TrimRight(s, "\n\r")
}

func fitStr(s string, w int) string {
	runes := []rune(s)
	if len(runes) > w {
		if w <= 1 {
			return "…"
		}
		return string(runes[:w-1]) + "…"
	}
	if len(runes) < w {
		return s + strings.Repeat(" ", w-len(runes))
	}
	return s
}

type lineGroup struct {
	op    gitdiff.LineOp
	lines []string
}

func groupLines(lines []gitdiff.Line) []lineGroup {
	var groups []lineGroup
	for _, l := range lines {
		text := trimLine(l.Line)
		if len(groups) > 0 && groups[len(groups)-1].op == l.Op {
			groups[len(groups)-1].lines = append(groups[len(groups)-1].lines, text)
		} else {
			groups = append(groups, lineGroup{op: l.Op, lines: []string{text}})
		}
	}
	return groups
}

func renderDiff(raw string, width int, filename string) (string, []int) {
	if width <= 0 {
		width = 80
	}
	files, _, err := gitdiff.Parse(strings.NewReader(raw))
	if err != nil || len(files) == 0 {
		return raw, nil
	}
	var b strings.Builder
	var hunkLines []int
	lineCount := 0
	for i, f := range files {
		if i > 0 {
			b.WriteByte('\n')
			lineCount++
		}
		renderFileDiff(&b, f, width, filename, &hunkLines, &lineCount)
	}
	return b.String(), hunkLines
}

func isNewFile(f *gitdiff.File) bool {
	return f.IsNew || f.OldName == "/dev/null" || f.OldName == ""
}

func isPurelyAdditive(f *gitdiff.File) bool {
	for _, frag := range f.TextFragments {
		for _, line := range frag.Lines {
			if line.Op == gitdiff.OpDelete {
				return false
			}
		}
	}
	return true
}

func renderFileDiff(b *strings.Builder, f *gitdiff.File, width int, filename string, hunkLines *[]int, lineCount *int) {
	name := f.NewName
	if name == "" {
		name = f.OldName
	}
	if filename != "" {
		name = filename
	}

	header := "── " + name + " "
	pad := width - len([]rune(header))
	if pad > 0 {
		header += strings.Repeat("─", pad)
	}
	b.WriteString(fileHdrSty.Render(header))
	b.WriteByte('\n')
	*lineCount++

	if f.IsBinary {
		b.WriteString(ctxDimSty.Render("  Binary file"))
		b.WriteByte('\n')
		*lineCount++
		return
	}

	newFile := isNewFile(f)
	addOnly := !newFile && isPurelyAdditive(f)
	hl := newHighlighter(name)

	if newFile || addOnly {
		for _, frag := range f.TextFragments {
			*hunkLines = append(*hunkLines, *lineCount)
			if !newFile && frag.Comment != "" {
				b.WriteString(hunkHdrSty.Render(frag.Comment))
				b.WriteByte('\n')
				*lineCount++
			}
			before := b.Len()
			renderSingleColumn(b, frag, width, hl, newFile)
			*lineCount += strings.Count(b.String()[before:], "\n")
		}
		return
	}

	for _, frag := range f.TextFragments {
		*hunkLines = append(*hunkLines, *lineCount)
		if frag.Comment != "" {
			b.WriteString(hunkHdrSty.Render(frag.Comment))
			b.WriteByte('\n')
			*lineCount++
		}
		before := b.Len()
		if width >= sideBySideMinWidth {
			renderSideBySide(b, frag, width, hl)
		} else {
			renderUnified(b, frag, width, hl)
		}
		*lineCount += strings.Count(b.String()[before:], "\n")
	}
}

func renderSideBySide(b *strings.Builder, frag *gitdiff.TextFragment, width int, hl *highlighter) {
	const numW = 4
	colW := (width - numW*2 - 5) / 2
	if colW < 10 {
		colW = 10
	}

	groups := groupLines(frag.Lines)
	oldNum := int(frag.OldPosition)
	newNum := int(frag.NewPosition)

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
		case gitdiff.OpContext:
			for _, text := range g.lines {
				emitRow(oldNum, text, bgNone, newNum, text, bgNone)
				oldNum++
				newNum++
			}
		case gitdiff.OpDelete:
			var addGrp *lineGroup
			if i+1 < len(groups) && groups[i+1].op == gitdiff.OpAdd {
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
		case gitdiff.OpAdd:
			for _, text := range g.lines {
				emitRow(0, "", bgNone, newNum, text, bgAdd)
				newNum++
			}
		}
	}
}

func renderSingleColumn(b *strings.Builder, frag *gitdiff.TextFragment, width int, hl *highlighter, newFile bool) {
	const numW = 4
	textW := width - numW - 2
	if textW < 10 {
		textW = 10
	}

	lineNum := int(frag.NewPosition)

	for _, line := range frag.Lines {
		text := trimLine(line.Line)
		chunks := wrapText(text, textW)

		bg := bgNone
		if !newFile && line.Op == gitdiff.OpAdd {
			bg = bgAdd
		}

		for ci, chunk := range chunks {
			if ci == 0 {
				b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, lineNum)))
				b.WriteString("  ")
			} else {
				b.WriteString(strings.Repeat(" ", numW+2))
			}
			b.WriteString(hl.renderLine(chunk, textW, bg))
			b.WriteByte('\n')
		}

		lineNum++
	}
}

func renderUnified(b *strings.Builder, frag *gitdiff.TextFragment, width int, hl *highlighter) {
	const numW = 4
	textW := width - numW*2 - 4
	if textW < 10 {
		textW = 10
	}

	oldNum := int(frag.OldPosition)
	newNum := int(frag.NewPosition)

	blankPrefix := strings.Repeat(" ", numW*2+4)

	for _, line := range frag.Lines {
		text := trimLine(line.Line)

		chunks := wrapText(text, textW)

		for ci, chunk := range chunks {
			switch line.Op {
			case gitdiff.OpContext:
				if ci == 0 {
					b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*d", numW, oldNum, numW, newNum)))
					b.WriteString("   ")
				} else {
					b.WriteString(blankPrefix)
				}
				b.WriteString(hl.renderLine(chunk, textW, bgNone))

			case gitdiff.OpDelete:
				if ci == 0 {
					b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*s", numW, oldNum, numW, "")))
					b.WriteString(delIndSty.Render(" -"))
					b.WriteByte(' ')
				} else {
					b.WriteString(blankPrefix)
				}
				b.WriteString(hl.renderLine(chunk, textW, bgDel))

			case gitdiff.OpAdd:
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

		switch line.Op {
		case gitdiff.OpContext:
			oldNum++
			newNum++
		case gitdiff.OpDelete:
			oldNum++
		case gitdiff.OpAdd:
			newNum++
		}
	}
}
