package main

import (
	"os"
	"os/exec"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type semLoadedMsg struct {
	data *semCache
	err  error
}

func loadSemData() tea.Msg {
	unstaged, err := runSemDiff()
	if err != nil {
		return semLoadedMsg{err: err}
	}
	var staged *semOutput
	if commitFrom == "" && !flagMain {
		staged, _ = runSemDiffStaged()
	}
	return semLoadedMsg{data: &semCache{unstaged: unstaged, staged: staged}}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.fullScreen {
			switch msg.String() {
			case "+", "=":
				m.contextLines += 4
				return m, m.openFullDiff()
			case "-":
				m.contextLines -= 4
				if m.contextLines < 0 {
					m.contextLines = 0
				}
				return m, m.openFullDiff()
			case "f":
				m.fullFile = !m.fullFile
				return m, m.openFullDiff()
			case "q", "esc":
				m.fullScreen = false
				return m, m.loadPreview()
			case "n":
				top := m.fullViewport.YOffset
				for _, line := range m.fullHunkLines {
					if line > top {
						m.fullViewport.SetYOffset(line)
						break
					}
				}
				return m, nil
			case "p":
				top := m.fullViewport.YOffset
				for i := len(m.fullHunkLines) - 1; i >= 0; i-- {
					if m.fullHunkLines[i] < top {
						m.fullViewport.SetYOffset(m.fullHunkLines[i])
						break
					}
				}
				return m, nil
			case "s":
				if semAvailable() {
					m.semantic = !m.semantic
					cfg.Semantic = &m.semantic
					saveConfig()
					if m.semantic && m.semData == nil && !m.semLoading {
						m.semLoading = true
						return m, loadSemData
					}
					return m, m.openFullDiff()
				}
				return m, nil
			case "e":
				f := m.selectedFile()
				if f == nil {
					return m, nil
				}
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vim"
				}
				c := exec.Command(editor, f.path)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return editorFinishedMsg{err}
				})
			default:
				var cmd tea.Cmd
				m.fullViewport, cmd = m.fullViewport.Update(msg)
				return m, cmd
			}
		}

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

		if m.themePicking {
			switch msg.String() {
			case "up", "k":
				if m.themeCursor > 0 {
					m.themeCursor--
					pal.chromaStyle = m.themeNames[m.themeCursor]
					return m, m.loadPreview()
				}
				return m, nil
			case "down", "j":
				if m.themeCursor < len(m.themeNames)-1 {
					m.themeCursor++
					pal.chromaStyle = m.themeNames[m.themeCursor]
					return m, m.loadPreview()
				}
				return m, nil
			case "enter":
				selected := m.themeNames[m.themeCursor]
				if darkMode {
					cfg.DarkTheme = selected
					darkPalette.chromaStyle = selected
				} else {
					cfg.LightTheme = selected
					lightPalette.chromaStyle = selected
				}
				pal.chromaStyle = selected
				saveConfig()
				m.themePicking = false
				return m, m.loadPreview()
			case "esc", "q":
				if darkMode {
					pal.chromaStyle = cfg.DarkTheme
				} else {
					pal.chromaStyle = cfg.LightTheme
				}
				m.themePicking = false
				return m, m.loadPreview()
			}
			return m, nil
		}

		switch msg.String() {
		case "+", "=":
			m.contextLines += 4
			return m, m.loadPreview()
		case "-":
			m.contextLines -= 4
			if m.contextLines < 0 {
				m.contextLines = 0
			}
			return m, m.loadPreview()
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
		case "ctrl+d":
			prev := m.cursor
			half := (m.height - 2) / 2
			if half < 1 {
				half = 1
			}
			m.moveCursorN(half)
			if m.cursor != prev {
				return m, m.loadPreview()
			}
			return m, nil
		case "ctrl+u":
			prev := m.cursor
			half := (m.height - 2) / 2
			if half < 1 {
				half = 1
			}
			m.moveCursorN(-half)
			if m.cursor != prev {
				return m, m.loadPreview()
			}
			return m, nil
		case "f":
			m.fullFile = !m.fullFile
			return m, m.loadPreview()
		case "enter":
			return m, m.openFullDiff()
		case "/":
			m.searching = true
			m.query = ""
			return m, nil
		case "t":
			darkMode = !darkMode
			applyTheme()
			return m, m.loadPreview()
		case "e":
			f := m.selectedFile()
			if f == nil {
				return m, nil
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}
			c := exec.Command(editor, f.path)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return editorFinishedMsg{err}
			})
		case "s":
			if semAvailable() {
				m.semantic = !m.semantic
				cfg.Semantic = &m.semantic
				saveConfig()
				if m.semantic && m.semData == nil && !m.semLoading {
					m.semLoading = true
					return m, loadSemData
				}
				return m, m.loadPreview()
			}
			return m, nil
		case "T":
			m.themePicking = true
			m.themeNames = nil
			for _, name := range styles.Names() {
				if isStyleDark(name) == darkMode {
					m.themeNames = append(m.themeNames, name)
				}
			}
			m.themeCursor = 0
			for i, name := range m.themeNames {
				if name == pal.chromaStyle {
					m.themeCursor = i
					break
				}
			}
			return m, nil
		}

	case tea.MouseMsg:
		if m.fullScreen {
			var cmd tea.Cmd
			m.fullViewport, cmd = m.fullViewport.Update(msg)
			return m, cmd
		}

		if m.dragging {
			if msg.Action == tea.MouseActionMotion {
				newW := clampTreeW(msg.X + 1)
				if newW != m.treeW && newW < m.width-20 {
					m.treeW = newW
					cfg.SidebarRatio = float64(m.treeW) / float64(m.width)
					vpW := m.width - m.treeW - 1
					if vpW < 20 {
						vpW = 20
					}
					m.viewport.Width = vpW
					return m, m.loadPreview()
				}
				return m, nil
			}
			if msg.Action == tea.MouseActionRelease {
				m.dragging = false
				saveConfig()
				return m, nil
			}
			return m, nil
		}

		onBorder := msg.X >= m.treeW-1 && msg.X <= m.treeW+1
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && onBorder {
			m.dragging = true
			return m, nil
		}

		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			if msg.X < m.treeW {
				delta := 3
				if msg.Button == tea.MouseButtonWheelUp {
					delta = -delta
				}
				m.scroll += delta
				maxScroll := len(m.filtered) - (m.height - 2)
				if maxScroll < 0 {
					maxScroll = 0
				}
				if m.scroll > maxScroll {
					m.scroll = maxScroll
				}
				if m.scroll < 0 {
					m.scroll = 0
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.X < m.treeW {
			clicked := m.scroll + (msg.Y - 1)
			if clicked >= 0 && clicked < len(m.filtered) {
				if m.allLines[m.filtered[clicked]].file != nil {
					prev := m.cursor
					m.cursor = clicked
					if m.cursor != prev {
						return m, m.loadPreview()
					}
				}
			}
			return m, nil
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeLayout()
		if !m.ready {
			m.ready = true
			return m, m.loadPreview()
		}
		if m.fullScreen {
			return m, m.openFullDiff()
		}
		return m, m.loadPreview()

	case diffLoadedMsg:
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		m.hunkLines = msg.hunkLines
		return m, nil

	case tickMsg:
		if commitFrom != "" {
			return m, nil
		}
		return m, tea.Batch(refreshFiles, tickCmd())

	case editorFinishedMsg:
		return m, refreshFiles

	case filesRefreshedMsg:
		if filesEqual(m.files, msg.files) {
			return m, nil
		}
		if m.semantic {
			m.semData = nil
		}
		selectedPath := ""
		if f := m.selectedFile(); f != nil {
			selectedPath = f.path
		}
		m.files = msg.files
		tree := buildTree(m.files)
		m.allLines = flattenTree(tree, 0)
		m.updateFilter()
		if selectedPath != "" {
			for i, idx := range m.filtered {
				if m.allLines[idx].file != nil && m.allLines[idx].file.path == selectedPath {
					m.cursor = i
					break
				}
			}
		}
		if m.semantic && m.semData == nil && !m.semLoading {
			m.semLoading = true
			return m, tea.Batch(loadSemData, m.loadPreview())
		}
		return m, m.loadPreview()

	case semLoadedMsg:
		m.semLoading = false
		if msg.err == nil && msg.data != nil {
			m.semData = msg.data
		}
		if m.fullScreen {
			return m, m.openFullDiff()
		}
		return m, m.loadPreview()

	case fullDiffLoadedMsg:
		m.fullScreen = true
		m.fullViewport = viewport.New(m.width, m.height-1)
		m.fullViewport.SetContent(msg.content)
		m.fullViewport.GotoTop()
		m.fullHunkLines = msg.hunkLines
		if f := m.selectedFile(); f != nil {
			m.fullFileName = f.path
		}
		return m, nil
	}

	return m, nil
}
