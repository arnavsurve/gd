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
	file   *fileStatus
	indent int
	name   string // display name (filename or dirname/)
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
			lines = append(lines, displayLine{
				file:   n.file,
				indent: indent,
				name:   n.name,
			})
		} else {
			lines = append(lines, displayLine{
				indent: indent,
				name:   n.name + "/",
			})
			lines = append(lines, flattenTree(n.children, indent+1)...)
		}
	}
	return lines
}

// --- Styles ---

var (
	dirStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	fileStyle      = lipgloss.NewStyle()
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("8"))
	stagedBadge    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	unstagedBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	untrackedBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	borderStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	searchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
)

// --- Bubbletea ---

type diffLoadedMsg struct{ content string }
type execFinishedMsg struct{ err error }

type model struct {
	allLines []displayLine // all tree lines
	filtered []int         // indices into allLines matching search
	cursor   int           // index into filtered
	scroll   int           // scroll offset for tree pane

	searching bool
	query     string

	viewport viewport.Model
	width    int
	height   int
	treeW    int // width of tree pane
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

	// Start cursor on first file line
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
		// Match files by path, always show parent dirs
		if line.file != nil && strings.Contains(strings.ToLower(line.file.path), q) {
			m.filtered = append(m.filtered, i)
		} else if line.file == nil && strings.Contains(strings.ToLower(line.name), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	// When filtering, also include parent directories of matched files
	if q != "" {
		dirSet := map[int]bool{}
		for _, idx := range m.filtered {
			if m.allLines[idx].file != nil {
				// Walk backwards to find parent dirs
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
		// Merge and re-sort
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
	width := m.width - m.treeW - 1
	if width < 40 {
		width = 40
	}
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
	visibleH := m.height - 2 // minus title and search/status line
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

	// Title
	title := titleStyle.Render("Changed Files")
	b.WriteString(title)
	b.WriteByte('\n')

	visibleH := m.height - 2 // title + search line
	if visibleH < 1 {
		visibleH = 1
	}

	end := m.scroll + visibleH
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	contentW := m.treeW - 1 // minus right border

	for i := m.scroll; i < end; i++ {
		lineIdx := m.filtered[i]
		line := m.allLines[lineIdx]
		indent := strings.Repeat("  ", line.indent)

		var rendered string
		if line.file == nil {
			rendered = indent + dirStyle.Render(line.name)
		} else {
			badge := ""
			if line.file.untracked {
				badge = untrackedBadge.Render("?")
			} else if line.file.staged && line.file.unstaged {
				badge = stagedBadge.Render("S") + unstagedBadge.Render("M")
			} else if line.file.staged {
				badge = stagedBadge.Render("S") + " "
			} else if line.file.unstaged {
				badge = unstagedBadge.Render("M") + " "
			}
			rendered = indent + badge + " " + fileStyle.Render(line.name)
		}

		if i == m.cursor {
			// Build plain text to measure width
			plain := indent
			if line.file == nil {
				plain += line.name
			} else {
				plain += line.file.statusLabel()
				if len(line.file.statusLabel()) == 1 {
					plain += " "
				}
				plain += " " + line.name
			}
			pad := contentW - len(plain)
			if pad < 0 {
				pad = 0
			}
			rendered = cursorStyle.Render(rendered + strings.Repeat(" ", pad))
		}

		// Truncate if wider than pane
		b.WriteString(rendered)
		b.WriteByte('\n')
	}

	// Pad remaining lines
	for i := end - m.scroll; i < visibleH; i++ {
		b.WriteByte('\n')
	}

	// Search / status line at bottom
	if m.searching {
		b.WriteString(searchStyle.Render("/" + m.query + "█"))
	} else if m.query != "" {
		b.WriteString(searchStyle.Render("/" + m.query) + borderStyle.Render("  (esc to clear)"))
	} else {
		b.WriteString(borderStyle.Render("/ search  enter view  q quit"))
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
				// Jump to first file match
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

	// Vertical border
	var border strings.Builder
	for i := 0; i < m.height; i++ {
		border.WriteString(borderStyle.Render("│"))
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
