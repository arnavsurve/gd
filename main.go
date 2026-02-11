package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var flagMain bool

// --- Git types ---

type fileStatus struct {
	path      string
	staged    bool
	unstaged  bool
	untracked bool
}

func (f fileStatus) statusLabel() string {
	if f.untracked {
		return "U"
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

// --- Git operations ---

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

func diffShellCmd(f fileStatus, fullFile bool, width int) string {
	ctx := ""
	if fullFile {
		ctx = "-U99999 "
	}

	var diffPart string
	if flagMain {
		diffPart = fmt.Sprintf("git diff %smain...HEAD -- %q", ctx, f.path)
	} else {
		var cmds []string
		if f.unstaged {
			cmds = append(cmds, fmt.Sprintf("git diff %s-- %q", ctx, f.path))
		}
		if f.staged {
			cmds = append(cmds, fmt.Sprintf("git diff --staged %s-- %q", ctx, f.path))
		}
		if f.untracked {
			cmds = append(cmds, fmt.Sprintf("git diff --no-index %s-- /dev/null %q 2>/dev/null", ctx, f.path))
		}
		if len(cmds) == 1 {
			diffPart = cmds[0]
		} else {
			diffPart = "{ " + strings.Join(cmds, "; ") + "; }"
		}
	}

	widthFlag := ""
	if width > 0 {
		widthFlag = fmt.Sprintf("--term-width %d ", width)
	}
	return diffPart + " | git-split-diffs --color " + widthFlag
}

// --- Tree ---

type treeNode struct {
	name     string
	file     *fileStatus
	children []*treeNode
}

type displayLine struct {
	text   string
	file   *fileStatus // nil = directory header
	indent int
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
		// directories first, then files
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
			lines = append(lines, displayLine{file: n.file, indent: indent})
		} else {
			lines = append(lines, displayLine{indent: indent, text: n.name + "/"})
			lines = append(lines, flattenTree(n.children, indent+1)...)
		}
	}
	return lines
}

// --- Styles ---

var (
	dirStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	fileStyle     = lipgloss.NewStyle()
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	stagedBadge   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	unstagedBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	untrackedBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	dividerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// --- Bubbletea ---

type diffLoadedMsg struct{ content string }
type execFinishedMsg struct{ err error }

type model struct {
	lines    []displayLine
	cursor   int
	scroll   int // scroll offset for tree pane
	viewport viewport.Model
	width    int
	height   int
	treeH    int
	ready    bool
}

func initialModel(files []fileStatus) model {
	tree := buildTree(files)
	lines := flattenTree(tree, 0)

	// Start cursor on first file line
	cursor := 0
	for i, l := range lines {
		if l.file != nil {
			cursor = i
			break
		}
	}

	return model{
		lines:    lines,
		cursor:   cursor,
		viewport: viewport.New(0, 0),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) selectedFile() *fileStatus {
	if m.cursor >= 0 && m.cursor < len(m.lines) {
		return m.lines[m.cursor].file
	}
	return nil
}

func (m model) loadPreview() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return func() tea.Msg { return diffLoadedMsg{content: ""} }
	}
	file := *f
	width := m.width
	return func() tea.Msg {
		cmd := diffShellCmd(file, false, width)
		out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
		return diffLoadedMsg{content: string(out)}
	}
}

func (m model) openFullDiff() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return nil
	}
	cmd := diffShellCmd(*f, true, 0) + " | less -RFX"
	c := exec.Command("sh", "-c", cmd)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *model) moveCursor(delta int) {
	n := len(m.lines)
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
	// Keep cursor in view
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+m.treeH {
		m.scroll = m.cursor - m.treeH + 1
	}
}

func (m model) renderTree() string {
	if m.treeH <= 0 {
		return ""
	}

	var b strings.Builder
	end := m.scroll + m.treeH
	if end > len(m.lines) {
		end = len(m.lines)
	}

	for i := m.scroll; i < end; i++ {
		line := m.lines[i]
		indent := strings.Repeat("  ", line.indent)

		var rendered string
		if line.file == nil {
			rendered = indent + dirStyle.Render(line.text)
		} else {
			badge := ""
			if line.file.untracked {
				badge = untrackedBadge.Render("?")
			} else if line.file.staged && line.file.unstaged {
				badge = stagedBadge.Render("S") + unstagedBadge.Render("M")
			} else if line.file.staged {
				badge = stagedBadge.Render("S")
			} else if line.file.unstaged {
				badge = unstagedBadge.Render("M")
			}
			name := fileStyle.Render(line.file.path[strings.LastIndex(line.file.path, "/")+1:])
			rendered = indent + badge + " " + name
		}

		if i == m.cursor {
			// Pad to full width for highlight
			plain := indent
			if line.file == nil {
				plain += line.text
			} else {
				plain += line.file.statusLabel() + " " + line.file.path[strings.LastIndex(line.file.path, "/")+1:]
			}
			pad := m.width - len(plain)
			if pad < 0 {
				pad = 0
			}
			rendered = selectedStyle.Render(rendered + strings.Repeat(" ", pad))
		}

		b.WriteString(rendered)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
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
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Tree takes up to 30% or 10 lines, whichever is smaller
		m.treeH = m.height * 30 / 100
		if m.treeH > 10 {
			m.treeH = 10
		}
		if m.treeH < 3 {
			m.treeH = 3
		}

		// Viewport gets the rest (minus 1 for divider)
		vpH := m.height - m.treeH - 1
		if vpH < 1 {
			vpH = 1
		}
		m.viewport.Width = m.width
		m.viewport.Height = vpH

		if !m.ready {
			m.ready = true
			return m, m.loadPreview()
		}

	case diffLoadedMsg:
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		return m, nil

	case execFinishedMsg:
		return m, m.loadPreview()
	}

	// Let viewport handle scroll events
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, vpCmd
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	tree := m.renderTree()
	divider := dividerStyle.Render(strings.Repeat("─", m.width))
	preview := m.viewport.View()

	return tree + "\n" + divider + "\n" + preview
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
