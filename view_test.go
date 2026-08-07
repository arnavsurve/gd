package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testModel(w, h int) model {
	files := []fileStatus{
		{path: "alpha.go", unstaged: true},
		{path: "bravo.go", unstaged: true},
		{path: "charlie.go", staged: true},
		{path: "delta.go", untracked: true},
	}
	m := initialModel(files, false)
	m.width, m.height = w, h
	m.resizeLayout()
	m.ready = true
	return m
}

// Bubble Tea's renderer keeps only the last `height` lines of a view, so a view
// even one line too tall silently scrolls the top row off screen and shifts
// every row up — which desyncs the sidebar's click-to-row math.
func TestViewFitsTerminalHeight(t *testing.T) {
	for _, h := range []int{10, 20, 51} {
		m := testModel(100, h)
		if got := lipgloss.Height(m.View()); got != h {
			t.Errorf("height %d: View() rendered %d lines, want %d", h, got, h)
		}
	}
}

func TestTreePaneFitsAboveStatusBar(t *testing.T) {
	m := testModel(100, 20)
	want := m.height - 1
	if got := lipgloss.Height(m.renderTree()); got != want {
		t.Errorf("renderTree() = %d lines, want %d (status bar takes the last row)", got, want)
	}
	m.themePicking = true
	if got := lipgloss.Height(m.renderThemePicker()); got != want {
		t.Errorf("renderThemePicker() = %d lines, want %d", got, want)
	}
}

// screenRow reports the row the given text is drawn on, as the terminal sees it.
func screenRow(t *testing.T, m model, text string) int {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if len(lines) > m.height {
		lines = lines[len(lines)-m.height:] // what Bubble Tea's renderer actually flushes
	}
	row := -1
	for i, line := range lines {
		if strings.Contains(line, text) {
			if row != -1 {
				t.Fatalf("%q appears on rows %d and %d", text, row, i)
			}
			row = i
		}
	}
	if row == -1 {
		t.Fatalf("%q not visible in rendered view", text)
	}
	return row
}

func TestClickSelectsRowUnderPointer(t *testing.T) {
	for _, path := range []string{"alpha.go", "bravo.go", "charlie.go", "delta.go"} {
		m := testModel(100, 20)
		row := screenRow(t, m, path)

		next, _ := m.Update(tea.MouseMsg{
			X:      1,
			Y:      row,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		})

		got := next.(model)
		sel := got.selectedFile()
		if sel == nil {
			t.Errorf("clicked %q on row %d, selected nothing", path, row)
			continue
		}
		if sel.path != path {
			t.Errorf("clicked %q on row %d, selected %q", path, row, sel.path)
		}
	}
}

func clickTree(m model, y int) model {
	next, _ := m.Update(tea.MouseMsg{
		X:      1,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	return next.(model)
}

func testModelPaths(paths []string, w, h int) model {
	files := make([]fileStatus, len(paths))
	for i, p := range paths {
		files[i] = fileStatus{path: p, unstaged: true}
	}
	m := initialModel(files, false)
	m.width, m.height = w, h
	m.resizeLayout()
	m.ready = true
	return m
}

// Rows that aren't sidebar entries must not select anything, even when the
// index arithmetic alone would land them on a real file. Height 5 gives a
// 3-row viewport: title on row 0, entries on rows 1-3, status bar on row 4.
func TestClickOutsideEntriesDoesNotSelect(t *testing.T) {
	paths := []string{"f0.go", "f1.go", "f2.go", "f3.go", "f4.go", "f5.go", "f6.go", "f7.go"}

	tests := []struct {
		name   string
		height int
		scroll int
		row    int
	}{
		{"title row while scrolled", 5, 1, 0},
		{"status bar row", 5, 1, 4},
		{"blank row below last entry", 20, 0, 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModelPaths(paths, 100, tc.height)
			m.scroll = tc.scroll
			m.cursor = 2 // distinct from any row the arithmetic could reach

			got := clickTree(m, tc.row)

			if got.cursor != 2 {
				t.Errorf("clicking row %d moved cursor 2 -> %d (%v), want it left alone",
					tc.row, got.cursor, got.selectedFile())
			}
		})
	}
}

func TestClickSelectsRowUnderPointerWhenScrolled(t *testing.T) {
	m := testModel(100, 5) // visibleH of 3, so the 4-file tree scrolls
	m.scroll = 1

	row := screenRow(t, m, "delta.go")
	next, _ := m.Update(tea.MouseMsg{
		X:      1,
		Y:      row,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	got := next.(model)
	sel := got.selectedFile()
	if sel == nil || sel.path != "delta.go" {
		t.Errorf("clicked delta.go on row %d with scroll=1, selected %v", row, sel)
	}
}
