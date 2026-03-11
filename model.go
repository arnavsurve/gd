package main

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const refreshInterval = 2 * time.Second

type editorFinishedMsg struct{ err error }

type tickMsg time.Time

type filesRefreshedMsg struct {
	files []fileStatus
}

type diffLoadedMsg struct {
	content   string
	hunkLines []int
}

type fullDiffLoadedMsg struct {
	content   string
	hunkLines []int
}

type model struct {
	allLines []displayLine
	files    []fileStatus
	filtered []int
	cursor   int
	scroll   int

	searching bool
	query     string

	themePicking bool
	themeNames   []string
	themeCursor  int

	viewport  viewport.Model
	hunkLines []int
	width     int
	height    int
	treeW     int
	ready     bool

	dragging bool

	fullScreen    bool
	fullViewport  viewport.Model
	fullHunkLines []int
	fullFileName  string
}

func initialModel(files []fileStatus) model {
	tree := buildTree(files)
	lines := flattenTree(tree, 0)

	m := model{
		allLines: lines,
		files:    files,
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
	if m.cursor >= 0 && m.cursor < len(m.filtered) && m.allLines[m.filtered[m.cursor]].file == nil {
		for i := m.cursor; i < len(m.filtered); i++ {
			if m.allLines[m.filtered[i]].file != nil {
				m.cursor = i
				return
			}
		}
		for i := m.cursor - 1; i >= 0; i-- {
			if m.allLines[m.filtered[i]].file != nil {
				m.cursor = i
				return
			}
		}
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func refreshFiles() tea.Msg {
	var files []fileStatus
	if flagMain {
		files, _ = getMainFiles()
	} else {
		files, _ = getChangedFiles()
	}
	return filesRefreshedMsg{files: files}
}

func (m model) selectedFile() *fileStatus {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.allLines[m.filtered[m.cursor]].file
	}
	return nil
}

func (m model) loadPreview() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return func() tea.Msg { return diffLoadedMsg{} }
	}
	file := *f
	vpW := m.width - m.treeW - 1
	if vpW < 40 {
		vpW = 40
	}
	return func() tea.Msg {
		raw := getDiffOutput(file, false)
		rendered, hunkLines := renderDiff(raw, vpW, file.path)
		return diffLoadedMsg{content: rendered, hunkLines: hunkLines}
	}
}

func (m model) openFullDiff() tea.Cmd {
	f := m.selectedFile()
	if f == nil {
		return nil
	}
	file := *f
	w := m.width
	return func() tea.Msg {
		raw := getDiffOutput(file, false)
		rendered, hunkLines := renderDiff(raw, w, file.path)
		return fullDiffLoadedMsg{content: rendered, hunkLines: hunkLines}
	}
}

func clampTreeW(w int) int {
	if w < 16 {
		w = 16
	}
	if w > 60 {
		w = 60
	}
	return w
}

func (m *model) resizeLayout() {
	if m.width == 0 {
		return
	}
	if cfg.SidebarRatio > 0 {
		m.treeW = clampTreeW(int(cfg.SidebarRatio * float64(m.width)))
	} else {
		m.treeW = clampTreeW(m.width / 5)
	}
	vpW := m.width - m.treeW - 1
	if vpW < 20 {
		vpW = 20
	}
	m.viewport.Width = vpW
	m.viewport.Height = m.height
	m.fullViewport.Width = m.width
	m.fullViewport.Height = m.height - 1
}

func (m *model) moveCursor(delta int) {
	n := len(m.filtered)
	if n == 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	next := m.cursor + dir
	for next >= 0 && next < n && m.allLines[m.filtered[next]].file == nil {
		next += dir
	}
	if next < 0 || next >= n {
		return
	}
	m.cursor = next
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
