package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var flagMain bool

const sideBySideMinWidth = 120

// ==================== Color Palette ====================

type palette struct {
	bgAdd      string
	bgDel      string
	lineNum    string
	hunkHdr    string
	fileHdr    string
	gutter     string
	addInd     string
	delInd     string
	ctxDim     string
	truncate   string
	dir        string
	file       string
	cursorFg   string
	cursorBg   string
	staged     string
	unstaged   string
	untracked  string
	border     string
	search     string
	title      string
	chromaStyle string
}

var darkPalette = palette{
	bgAdd:      "#1a2b1f",
	bgDel:      "#2b1d1e",
	lineNum:    "#484f58",
	hunkHdr:    "#79c0ff",
	fileHdr:    "#e6edf3",
	gutter:     "#30363d",
	addInd:     "#3fb950",
	delInd:     "#f85149",
	ctxDim:     "#8b949e",
	truncate:   "#484f58",
	dir:        "#79c0ff",
	file:       "#e6edf3",
	cursorFg:   "#e6edf3",
	cursorBg:   "#30363d",
	staged:     "#3fb950",
	unstaged:   "#d29922",
	untracked:  "#484f58",
	border:     "#30363d",
	search:     "#79c0ff",
	title:      "#e6edf3",
	chromaStyle: "tokyonight-night",
}

var lightPalette = palette{
	bgAdd:      "#e8f5e9",
	bgDel:      "#fce8e6",
	lineNum:    "#57606a",
	hunkHdr:    "#0969da",
	fileHdr:    "#1f2328",
	gutter:     "#d0d7de",
	addInd:     "#1a7f37",
	delInd:     "#cf222e",
	ctxDim:     "#656d76",
	truncate:   "#57606a",
	dir:        "#0969da",
	file:       "#1f2328",
	cursorFg:   "#1f2328",
	cursorBg:   "#ddf4ff",
	staged:     "#1a7f37",
	unstaged:   "#9a6700",
	untracked:  "#57606a",
	border:     "#d0d7de",
	search:     "#0969da",
	title:      "#1f2328",
	chromaStyle: "tokyonight-day",
}

// Active palette and styles, set in init()
var pal palette

var (
	lineNumSty lipgloss.Style
	hunkHdrSty lipgloss.Style
	fileHdrSty lipgloss.Style
	gutterSty  lipgloss.Style
	addIndSty  lipgloss.Style
	delIndSty  lipgloss.Style
	ctxDimSty  lipgloss.Style
	dirSty     lipgloss.Style
	fileSty    lipgloss.Style
	cursorSty  lipgloss.Style
	stagedBadge lipgloss.Style
	unstBadge  lipgloss.Style
	untrkBadge lipgloss.Style
	borderSty  lipgloss.Style
	searchSty  lipgloss.Style
	titleSty   lipgloss.Style
)

var bgColors map[diffBg]string

var darkMode bool

func initTheme() {
	darkMode = termenv.HasDarkBackground()
	applyTheme()
}

func applyTheme() {
	if darkMode {
		pal = darkPalette
	} else {
		pal = lightPalette
	}

	lineNumSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.lineNum))
	hunkHdrSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.hunkHdr)).Faint(true)
	fileHdrSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.fileHdr))
	gutterSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.gutter))
	addIndSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.addInd))
	delIndSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.delInd))
	ctxDimSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.ctxDim))
	dirSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.dir))
	fileSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.file))
	cursorSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.cursorFg)).Background(lipgloss.Color(pal.cursorBg))
	stagedBadge = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.staged))
	unstBadge = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.unstaged))
	untrkBadge = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.untracked))
	borderSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.border))
	searchSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.search))
	titleSty = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.title))

	bgColors = map[diffBg]string{
		bgNone: "",
		bgAdd:  pal.bgAdd,
		bgDel:  pal.bgDel,
	}
}

func isStyleDark(name string) bool {
	style := styles.Get(name)
	if style == nil {
		return true
	}
	bg := style.Get(chroma.Background).Background
	if !bg.IsSet() {
		return true
	}
	r := bg.Red()
	g := bg.Green()
	b := bg.Blue()
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	return lum < 128
}

// ==================== Config ====================

type config struct {
	DarkTheme  string `json:"darkTheme"`
	LightTheme string `json:"lightTheme"`
}

var cfg config

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "gd", "config.json")
}

func loadConfig() {
	cfg = config{
		DarkTheme:  darkPalette.chromaStyle,
		LightTheme: lightPalette.chromaStyle,
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	var loaded config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	if loaded.DarkTheme != "" {
		cfg.DarkTheme = loaded.DarkTheme
		darkPalette.chromaStyle = loaded.DarkTheme
	}
	if loaded.LightTheme != "" {
		cfg.LightTheme = loaded.LightTheme
		lightPalette.chromaStyle = loaded.LightTheme
	}
}

func saveConfig() {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(configPath())
	os.MkdirAll(dir, 0o755)
	os.WriteFile(configPath(), data, 0o644)
}

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
	out, err := exec.Command("git", "status", "--porcelain", "-uall").Output()
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

// ==================== Syntax Highlighting ====================

type highlighter struct {
	lexer chroma.Lexer
	style *chroma.Style
}

func newHighlighter(filename string) *highlighter {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(pal.chromaStyle)
	if style == nil {
		style = styles.Fallback
	}

	return &highlighter{lexer: lexer, style: style}
}

type diffBg int

const (
	bgNone diffBg = iota
	bgAdd
	bgDel
)

func (h *highlighter) renderLine(text string, w int, bg diffBg) string {
	runes := []rune(text)
	visW := len(runes)

	bgColor := bgColors[bg]

	iter, err := h.lexer.Tokenise(nil, text)
	if err != nil {
		// Fallback: plain text with bg
		s := lipgloss.NewStyle()
		if bgColor != "" {
			s = s.Background(lipgloss.Color(bgColor))
		}
		return s.Render(fitStr(text, w))
	}

	var b strings.Builder
	for _, tok := range iter.Tokens() {
		val := strings.TrimRight(tok.Value, "\n\r")
		if val == "" {
			continue
		}
		entry := h.style.Get(tok.Type)
		s := lipgloss.NewStyle()
		if entry.Colour.IsSet() {
			s = s.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		if bgColor != "" {
			s = s.Background(lipgloss.Color(bgColor))
		}
		if entry.Bold == chroma.Yes {
			s = s.Bold(true)
		}
		if entry.Italic == chroma.Yes {
			s = s.Italic(true)
		}
		b.WriteString(s.Render(val))
	}

	// Pad remaining width with background
	pad := w - visW
	if pad > 0 {
		s := lipgloss.NewStyle()
		if bgColor != "" {
			s = s.Background(lipgloss.Color(bgColor))
		}
		b.WriteString(s.Render(strings.Repeat(" ", pad)))
	}

	return b.String()
}

// ==================== Diff Rendering ====================

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

// wrapText splits text into chunks of at most w runes.
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

	hl := newHighlighter(name)

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
	// [lnum numW] [space 1] [left colW] [ │  3] [rnum numW] [space 1] [right colW]
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

func renderUnified(b *strings.Builder, frag *gitdiff.TextFragment, width int, hl *highlighter) {
	const numW = 4
	// [oldnum numW] [space] [newnum numW] [space] [indicator 1] [space] [text]
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

// ==================== TUI Model ====================

type editorFinishedMsg struct{ err error }

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

	fullScreen     bool
	fullViewport   viewport.Model
	fullHunkLines  []int
	fullFileName   string
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
	// Ensure cursor lands on a file, not a directory
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
	// Skip directory entries – only land on files
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

	modeLabel := "dark"
	if !darkMode {
		modeLabel = "light"
	}
	b.WriteString(fitStr("Theme ("+modeLabel+") ⏎ select esc cancel", contentW))

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

		// Truncate or pad to exactly contentW
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

	if m.searching {
		b.WriteString(fitStr("/"+m.query+"█", contentW))
	} else if m.query != "" {
		b.WriteString(fitStr("/"+m.query+"  esc clear", contentW))
	} else {
		b.WriteString(fitStr("/ search ⏎ view e edit q quit", contentW))
	}

	return b.String()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.fullScreen {
			switch msg.String() {
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
		m.treeW = m.width / 5
		if m.treeW < 16 {
			m.treeW = 16
		}
		if m.treeW > 40 {
			m.treeW = 40
		}
		vpW := m.width - m.treeW - 1
		if vpW < 20 {
			vpW = 20
		}
		m.viewport.Width = vpW
		m.viewport.Height = m.height
		m.fullViewport.Width = m.width
		m.fullViewport.Height = m.height - 1
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

	case editorFinishedMsg:
		return m, func() tea.Msg {
			var files []fileStatus
			if flagMain {
				files, _ = getMainFiles()
			} else {
				files, _ = getChangedFiles()
			}
			return filesRefreshedMsg{files: files}
		}

	case filesRefreshedMsg:
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

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.fullScreen {
		statusBar := borderSty.Render(m.fullFileName) +
			borderSty.Render("  n/p hunk  ctrl+u/d page up/down  q back")
		return m.fullViewport.View() + "\n" + statusBar
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

	loadConfig()
	initTheme()

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

	p := tea.NewProgram(initialModel(files), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
