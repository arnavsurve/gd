package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderThemePicker() string {
	var b strings.Builder
	contentW := m.treeW - 1
	b.WriteString(titleSty.Render(fitStr("Syntax Theme", contentW)))
	b.WriteByte('\n')

	visibleH := m.height - 2
	if visibleH < 1 {
		visibleH = 1
	}

	scroll := 0
	if m.themeCursor >= visibleH {
		scroll = m.themeCursor - visibleH + 1
	}
	end := scroll + visibleH
	if end > len(m.themeNames) {
		end = len(m.themeNames)
	}

	for i := scroll; i < end; i++ {
		name := m.themeNames[i]
		display := fitStr(name, contentW)
		if i == m.themeCursor {
			b.WriteString(cursorSty.Render(display))
		} else {
			b.WriteString(fileSty.Render(display))
		}
		b.WriteByte('\n')
	}

	blank := strings.Repeat(" ", contentW)
	for i := end - scroll; i < visibleH; i++ {
		b.WriteString(blank)
		b.WriteByte('\n')
	}

	return b.String()
}

func (m model) renderTree() string {
	if m.themePicking {
		return m.renderThemePicker()
	}
	var b strings.Builder
	contentW := m.treeW - 1
	title := fitStr("Changed Files", contentW)
	b.WriteString(titleSty.Render(title))
	b.WriteByte('\n')

	visibleH := m.height - 2
	if visibleH < 1 {
		visibleH = 1
	}
	end := m.scroll + visibleH
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.scroll; i < end; i++ {
		lineIdx := m.filtered[i]
		line := m.allLines[lineIdx]
		indent := strings.Repeat("  ", line.indent)

		var plain string
		var rendered string
		if line.file == nil {
			plain = indent + line.name
			rendered = indent + dirSty.Render(line.name)
		} else {
			badge := ""
			badgePlain := ""
			if line.file.untracked {
				badge = untrkBadge.Render("?")
				badgePlain = "?"
			} else if line.file.staged && line.file.unstaged {
				badge = stagedBadge.Render("S") + unstBadge.Render("M")
				badgePlain = "SM"
			} else if line.file.staged {
				badge = stagedBadge.Render("S") + " "
				badgePlain = "S "
			} else if line.file.unstaged {
				badge = unstBadge.Render("M") + " "
				badgePlain = "M "
			}
			plain = indent + badgePlain + " " + line.name
			rendered = indent + badge + " " + fileSty.Render(line.name)
		}

		runes := []rune(plain)
		if len(runes) > contentW {
			truncPlain := string(runes[:contentW-1]) + "…"
			if i == m.cursor {
				rendered = cursorSty.Render(truncPlain)
			} else {
				rendered = truncPlain
			}
		} else {
			padN := contentW - len(runes)
			if i == m.cursor {
				rendered = cursorSty.Render(rendered + strings.Repeat(" ", padN))
			} else {
				rendered = rendered + strings.Repeat(" ", padN)
			}
		}

		b.WriteString(rendered)
		b.WriteByte('\n')
	}

	blank := strings.Repeat(" ", contentW)
	for i := end - m.scroll; i < visibleH; i++ {
		b.WriteString(blank)
		b.WriteByte('\n')
	}

	return b.String()
}

func (m model) renderStatusBar() string {
	var text string
	if m.fullScreen {
		text = m.fullFileName + "  n/p hunk  f full  +/- context  e edit  t theme  T pick  q back"
	} else if m.themePicking {
		modeLabel := "dark"
		if !darkMode {
			modeLabel = "light"
		}
		text = "Theme (" + modeLabel + ")  ⏎ select  esc cancel"
	} else if m.searching {
		text = "/" + m.query + "█"
	} else if m.query != "" {
		text = "/" + m.query + "  esc clear"
	} else {
		text = "/ search  ⏎ view  e edit  f full  +/- context  t theme  T pick  q quit"
	}
	return borderSty.Render(fitStr(text, m.width))
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	statusBar := m.renderStatusBar()

	if m.fullScreen {
		return m.fullViewport.View() + "\n" + statusBar
	}

	treeView := m.renderTree()

	contentH := m.height - 1
	var border strings.Builder
	for i := 0; i < contentH; i++ {
		border.WriteString(borderSty.Render("│"))
		if i < contentH-1 {
			border.WriteByte('\n')
		}
	}

	diffView := m.viewport.View()
	main := lipgloss.JoinHorizontal(lipgloss.Top, treeView, border.String(), diffView)
	return main + "\n" + statusBar
}
