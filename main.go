package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var flagMain bool

const sideBySideMinWidth = 120

// ==================== Git Types ====================

type fileStatus struct {
	path      string
	staged    bool
	unstaged  bool
	untracked bool
}

func (f fileStatus) statusLabel() string {
	if f.untracked {
		return "?"
	}
	var s string
	if f.staged {
		s += "S"
	}
	if f.unstaged {
		s += "M"
	}
	return s
}

// ==================== Git Operations ====================

func getChangedFiles() ([]fileStatus, error) {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	seen := map[string]*fileStatus{}
	var order []string

	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		x, y := line[0], line[1]
		path := line[3:]
		if idx := strings.Index(path, " -> "); idx != -1 {
			path = path[idx+4:]
		}

		fs, ok := seen[path]
		if !ok {
			fs = &fileStatus{path: path}
			seen[path] = fs
			order = append(order, path)
		}
		if x == '?' && y == '?' {
			fs.untracked = true
		} else {
			if x != ' ' && x != '?' {
				fs.staged = true
			}
			if y != ' ' && y != '?' {
				fs.unstaged = true
			}
		}
	}

	files := make([]fileStatus, 0, len(order))
	for _, p := range order {
		files = append(files, *seen[p])
	}
	return files, nil
}

func getMainFiles() ([]fileStatus, error) {
	out, err := exec.Command("git", "diff", "--name-only", "main...HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only main...HEAD: %w", err)
	}
	var files []fileStatus
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			files = append(files, fileStatus{path: line})
		}
	}
	return files, nil
}

func getDiffOutput(f fileStatus, fullFile bool) string {
	ctx := ""
	if fullFile {
		ctx = "-U99999 "
	}
	var cmds []string
	if flagMain {
		cmds = append(cmds, fmt.Sprintf("git diff %smain...HEAD -- %q", ctx, f.path))
	} else {
		if f.unstaged {
			cmds = append(cmds, fmt.Sprintf("git diff %s-- %q", ctx, f.path))
		}
		if f.staged {
			cmds = append(cmds, fmt.Sprintf("git diff --staged %s-- %q", ctx, f.path))
		}
		if f.untracked {
			cmds = append(cmds, fmt.Sprintf("git diff --no-index %s-- /dev/null %q 2>/dev/null", ctx, f.path))
		}
	}

	var cmd string
	if len(cmds) == 1 {
		cmd = cmds[0]
	} else {
		cmd = "{ " + strings.Join(cmds, "; ") + "; }"
	}

	out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
	return string(out)
}

// ==================== Tree ====================

type treeNode struct {
	name     string
	file     *fileStatus
	children []*treeNode
}

type displayLine struct {
	file   *fileStatus
	indent int
	name   string
}

func buildTree(files []fileStatus) []*treeNode {
	root := &treeNode{}
	for i := range files {
		f := &files[i]
		parts := strings.Split(f.path, "/")
		cur := root
		for j, part := range parts {
			if j == len(parts)-1 {
				cur.children = append(cur.children, &treeNode{name: part, file: f})
			} else {
				var found *treeNode
				for _, ch := range cur.children {
					if ch.file == nil && ch.name == part {
						found = ch
						break
					}
				}
				if found == nil {
					found = &treeNode{name: part}
					cur.children = append(cur.children, found)
				}
				cur = found
			}
		}
	}
	sortTree(root.children)
	return root.children
}

func sortTree(nodes []*treeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		iDir := nodes[i].file == nil
		jDir := nodes[j].file == nil
		if iDir != jDir {
			return iDir
		}
		return nodes[i].name < nodes[j].name
	})
	for _, n := range nodes {
		if n.file == nil {
			sortTree(n.children)
		}
	}
}

func flattenTree(nodes []*treeNode, indent int) []displayLine {
	var lines []displayLine
	for _, n := range nodes {
		if n.file != nil {
			lines = append(lines, displayLine{file: n.file, indent: indent, name: n.name})
		} else {
			lines = append(lines, displayLine{indent: indent, name: n.name + "/"})
			lines = append(lines, flattenTree(n.children, indent+1)...)
		}
	}
	return lines
}

// ==================== Diff Rendering ====================

var (
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	delStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	ctxStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	lineNumSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	hunkHdrSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Faint(true)
	fileHdrSty  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	addBgSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("22"))
	delBgSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Background(lipgloss.Color("52"))
	gutterSty   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func trimLine(s string) string {
	return strings.TrimRight(s, "\n\r")
}

func truncStr(s string, w int) string {
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

func padStr(s string, w int) string {
	runes := []rune(s)
	if len(runes) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(runes))
}

func fitStr(s string, w int) string {
	return padStr(truncStr(s, w), w)
}

// lineGroup groups consecutive lines with the same operation.
type lineGroup struct {
	op    gitdiff.LineOp
	lines []string
}

func groupLines(lines []gitdiff.Line) []lineGroup {
	var groups []lineGroup
	for _, l := range lines {
		text := expandTabs(trimLine(l.Line))
		if len(groups) > 0 && groups[len(groups)-1].op == l.Op {
			groups[len(groups)-1].lines = append(groups[len(groups)-1].lines, text)
		} else {
			groups = append(groups, lineGroup{op: l.Op, lines: []string{text}})
		}
	}
	return groups
}

func renderDiff(raw string, width int) string {
	if width <= 0 {
		width = 80
	}
	files, _, err := gitdiff.Parse(strings.NewReader(raw))
	if err != nil || len(files) == 0 {
		return raw
	}

	var b strings.Builder
	for i, f := range files {
		if i > 0 {
			b.WriteByte('\n')
		}
		renderFileDiff(&b, f, width)
	}
	return b.String()
}

func renderFileDiff(b *strings.Builder, f *gitdiff.File, width int) {
	name := f.NewName
	if name == "" {
		name = f.OldName
	}
	header := "── " + name + " "
	pad := width - len([]rune(header))
	if pad > 0 {
		header += strings.Repeat("─", pad)
	}
	b.WriteString(fileHdrSty.Render(header))
	b.WriteByte('\n')

	if f.IsBinary {
		b.WriteString(ctxStyle.Render("  Binary file"))
		b.WriteByte('\n')
		return
	}

	for _, frag := range f.TextFragments {
		if frag.Comment != "" {
			b.WriteString(hunkHdrSty.Render(frag.Comment))
			b.WriteByte('\n')
		}
		if width >= sideBySideMinWidth {
			renderSideBySide(b, frag, width)
		} else {
			renderUnified(b, frag, width)
		}
	}
}

func renderSideBySide(b *strings.Builder, frag *gitdiff.TextFragment, width int) {
	// Layout: [lnum 4] [space] [left colW] [gutter 3] [rnum 4] [space] [right colW]
	// 4 + 1 + colW + 3 + 4 + 1 + colW = width => colW = (width - 13) / 2
	const numW = 4
	colW := (width - 13) / 2
	if colW < 10 {
		colW = 10
	}

	groups := groupLines(frag.Lines)
	oldNum := int(frag.OldPosition)
	newNum := int(frag.NewPosition)

	emitRow := func(lNum int, lText string, lSty lipgloss.Style, rNum int, rText string, rSty lipgloss.Style) {
		// Left line number
		if lNum > 0 {
			b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, lNum)))
		} else {
			b.WriteString(lineNumSty.Render(strings.Repeat(" ", numW)))
		}
		b.WriteByte(' ')

		// Left text
		b.WriteString(lSty.Render(fitStr(lText, colW)))

		// Gutter
		b.WriteString(gutterSty.Render(" │ "))

		// Right line number
		if rNum > 0 {
			b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d", numW, rNum)))
		} else {
			b.WriteString(lineNumSty.Render(strings.Repeat(" ", numW)))
		}
		b.WriteByte(' ')

		// Right text
		b.WriteString(rSty.Render(fitStr(rText, colW)))
		b.WriteByte('\n')
	}

	for i := 0; i < len(groups); i++ {
		g := groups[i]
		switch g.op {
		case gitdiff.OpContext:
			for _, text := range g.lines {
				emitRow(oldNum, text, ctxStyle, newNum, text, ctxStyle)
				oldNum++
				newNum++
			}

		case gitdiff.OpDelete:
			// Check if next group is add (modification pair)
			var addGroup *lineGroup
			if i+1 < len(groups) && groups[i+1].op == gitdiff.OpAdd {
				addGroup = &groups[i+1]
				i++
			}

			maxLen := len(g.lines)
			if addGroup != nil && len(addGroup.lines) > maxLen {
				maxLen = len(addGroup.lines)
			}

			for j := 0; j < maxLen; j++ {
				var lNum int
				var lText string
				lSty := delBgSty
				var rNum int
				var rText string
				rSty := addBgSty

				if j < len(g.lines) {
					lNum = oldNum
					lText = g.lines[j]
					oldNum++
				} else {
					lSty = ctxStyle
				}

				if addGroup != nil && j < len(addGroup.lines) {
					rNum = newNum
					rText = addGroup.lines[j]
					newNum++
				} else {
					rSty = ctxStyle
				}

				emitRow(lNum, lText, lSty, rNum, rText, rSty)
			}

		case gitdiff.OpAdd:
			// Lone add (not preceded by delete)
			for _, text := range g.lines {
				emitRow(0, "", ctxStyle, newNum, text, addBgSty)
				newNum++
			}
		}
	}
}

func renderUnified(b *strings.Builder, frag *gitdiff.TextFragment, width int) {
	const numW = 4
	// Layout: [oldnum 4] [newnum 4] [space] [op 1] [space] [text]
	textW := width - numW*2 - 4
	if textW < 10 {
		textW = 10
	}

	oldNum := int(frag.OldPosition)
	newNum := int(frag.NewPosition)

	for _, line := range frag.Lines {
		text := expandTabs(trimLine(line.Line))

		switch line.Op {
		case gitdiff.OpContext:
			b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*d", numW, oldNum, numW, newNum)))
			b.WriteString(ctxStyle.Render("   " + truncStr(text, textW)))
			oldNum++
			newNum++

		case gitdiff.OpDelete:
			b.WriteString(lineNumSty.Render(fmt.Sprintf("%*d %*s", numW, oldNum, numW, "")))
			b.WriteString(delStyle.Render(" - " + truncStr(text, textW)))
			oldNum++

		case gitdiff.OpAdd:
			b.WriteString(lineNumSty.Render(fmt.Sprintf("%*s %*d", numW, "", numW, newNum)))
			b.WriteString(addStyle.Render(" + " + truncStr(text, textW)))
			newNum++
		}
		b.WriteByte('\n')
	}
}

// ==================== TUI Styles ====================

var (
	dirSty       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	fileSty      = lipgloss.NewStyle()
	cursorSty    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("8"))
	stagedBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	unstBadge    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	untrkBadge   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	borderSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	searchSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	titleSty     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
)

// ==================== TUI Model ====================

type diffLoadedMsg struct{ content string }
type execFinishedMsg struct{ err error }

type model struct {
	allLines []displayLine
	filtered []int
	cursor   int
	scroll   int

	searching bool
	query     string

	viewport viewport.Model
	width    int
	height   int
	treeW    int
	ready    bool
}

func initialModel(files []fileStatus) model {
	tree := buildTree(files)
	lines := flattenTree(tree, 0)

	m := model{
		allLines: lines,
		viewport: viewport.New(0, 0),
	}
	m.updateFilter()

	for i, idx := range m.filtered {
		if m.allLines[idx].file != nil {
			m.cursor = i
			break
		}
	}

	return m
}

func (m *model) updateFilter() {
	m.filtered = nil
	q := strings.ToLower(m.query)

	for i, line := range m.allLines {
		if q == "" {
			m.filtered = append(m.filtered, i)
			continue
		}
		if line.file != nil && strings.Contains(strings.ToLower(line.file.path), q) {
			m.filtered = append(m.filtered, i)
		} else if line.file == nil && strings.Contains(strings.ToLower(line.name), q) {
			m.filtered = append(m.filtered, i)
		}
	}

	// Include parent directories of matched files
	if q != "" {
		dirSet := map[int]bool{}
		for _, idx := range m.filtered {
			if m.allLines[idx].file != nil {
				for j := idx - 1; j >= 0; j-- {
					if m.allLines[j].file == nil && m.allLines[j].indent < m.allLines[idx].indent {
						dirSet[j] = true
						if m.allLines[j].indent == 0 {
							break
						}
					}
				}
			}
		}
		existing := map[int]bool{}
		for _, idx := range m.filtered {
			existing[idx] = true
		}
		for idx := range dirSet {
			if !existing[idx] {
				m.filtered = append(m.filtered, idx)
			}
		}
		sort.Ints(m.filtered)
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) selectedFile() *fileStatus {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.allLines[m.filtered[m.cursor]].file
	}
	return nil
}

func (m model) loadPreview() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return func() tea.Msg { return diffLoadedMsg{content: ""} }
	}
	file := *f
	vpW := m.width - m.treeW - 1
	if vpW < 40 {
		vpW = 40
	}
	return func() tea.Msg {
		raw := getDiffOutput(file, false)
		rendered := renderDiff(raw, vpW)
		return diffLoadedMsg{content: rendered}
	}
}

func (m model) openFullDiff() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return nil
	}
	raw := getDiffOutput(*f, true)
	rendered := renderDiff(raw, m.width)

	c := exec.Command("less", "-RFX")
	c.Stdin = strings.NewReader(rendered)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *model) moveCursor(delta int) {
	n := len(m.filtered)
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	visibleH := m.height - 2
	if visibleH < 1 {
		visibleH = 1
	}
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+visibleH {
		m.scroll = m.cursor - visibleH + 1
	}
}

func (m model) renderTree() string {
	var b strings.Builder

	b.WriteString(titleSty.Render("Changed Files"))
	b.WriteByte('\n')

	visibleH := m.height - 2
	if visibleH < 1 {
		visibleH = 1
	}

	end := m.scroll + visibleH
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	contentW := m.treeW - 1

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

		if i == m.cursor {
			padN := contentW - len([]rune(plain))
			if padN < 0 {
				padN = 0
			}
			rendered = cursorSty.Render(rendered + strings.Repeat(" ", padN))
		}

		b.WriteString(truncStr(rendered, contentW))
		b.WriteByte('\n')
	}

	// Pad remaining
	for i := end - m.scroll; i < visibleH; i++ {
		b.WriteByte('\n')
	}

	// Status line
	if m.searching {
		b.WriteString(searchSty.Render("/" + m.query + "█"))
	} else if m.query != "" {
		b.WriteString(searchSty.Render("/" + m.query) + borderSty.Render("  esc clear"))
	} else {
		b.WriteString(borderSty.Render("/ search  ⏎ view  q quit"))
	}

	return b.String()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "enter":
				m.searching = false
				for i, idx := range m.filtered {
					if m.allLines[idx].file != nil {
						m.cursor = i
						break
					}
				}
				return m, m.loadPreview()
			case "esc":
				m.searching = false
				m.query = ""
				m.updateFilter()
				return m, m.loadPreview()
			case "backspace":
				if len(m.query) > 0 {
					m.query = m.query[:len(m.query)-1]
					m.updateFilter()
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.query += msg.String()
					m.updateFilter()
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.query != "" {
				m.query = ""
				m.updateFilter()
				return m, m.loadPreview()
			}
			return m, tea.Quit
		case "up", "k":
			prev := m.cursor
			m.moveCursor(-1)
			if m.cursor != prev {
				return m, m.loadPreview()
			}
			return m, nil
		case "down", "j":
			prev := m.cursor
			m.moveCursor(1)
			if m.cursor != prev {
				return m, m.loadPreview()
			}
			return m, nil
		case "enter":
			return m, m.openFullDiff()
		case "/":
			m.searching = true
			m.query = ""
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.treeW = m.width * 30 / 100
		if m.treeW < 30 {
			m.treeW = 30
		}
		if m.treeW > 50 {
			m.treeW = 50
		}

		vpW := m.width - m.treeW - 1
		if vpW < 20 {
			vpW = 20
		}

		m.viewport.Width = vpW
		m.viewport.Height = m.height

		if !m.ready {
			m.ready = true
			return m, m.loadPreview()
		}
		return m, m.loadPreview()

	case diffLoadedMsg:
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		return m, nil

	case execFinishedMsg:
		return m, m.loadPreview()
	}

	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	treeView := m.renderTree()

	var border strings.Builder
	for i := 0; i < m.height; i++ {
		border.WriteString(borderSty.Render("│"))
		if i < m.height-1 {
			border.WriteByte('\n')
		}
	}

	diffView := m.viewport.View()

	return lipgloss.JoinHorizontal(lipgloss.Top, treeView, border.String(), diffView)
}

func main() {
	flag.BoolVar(&flagMain, "main", false, "diff against main branch")
	flag.Parse()

	var files []fileStatus
	var err error
	if flagMain {
		files, err = getMainFiles()
	} else {
		files, err = getChangedFiles()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("No changes.")
		return
	}

	p := tea.NewProgram(initialModel(files), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
