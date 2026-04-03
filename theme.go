package main

import (
	"os/exec"
	"runtime"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type palette struct {
	bgAdd       string
	bgDel       string
	bgMoved     string
	lineNum     string
	hunkHdr     string
	fileHdr     string
	gutter      string
	addInd      string
	delInd      string
	ctxDim      string
	truncate    string
	dir         string
	file        string
	cursorFg    string
	cursorBg    string
	staged      string
	unstaged    string
	untracked   string
	border      string
	search      string
	title       string
	movedHdr    string
	renamedHdr  string
	summaryFg   string
	chromaStyle string
}

var darkPalette = palette{
	bgAdd:       "#1a2b1f",
	bgDel:       "#2b1d1e",
	bgMoved:     "#1a1f2b",
	lineNum:     "#484f58",
	hunkHdr:     "#79c0ff",
	fileHdr:     "#e6edf3",
	gutter:      "#30363d",
	addInd:      "#3fb950",
	delInd:      "#f85149",
	ctxDim:      "#8b949e",
	truncate:    "#484f58",
	dir:         "#79c0ff",
	file:        "#e6edf3",
	cursorFg:    "#e6edf3",
	cursorBg:    "#30363d",
	staged:      "#3fb950",
	unstaged:    "#d29922",
	untracked:   "#484f58",
	border:      "#30363d",
	search:      "#79c0ff",
	title:       "#e6edf3",
	movedHdr:    "#58a6ff",
	renamedHdr:  "#d29922",
	summaryFg:   "#8b949e",
	chromaStyle: "tokyonight-night",
}

var lightPalette = palette{
	bgAdd:       "#e8f5e9",
	bgDel:       "#fce8e6",
	bgMoved:     "#ddf4ff",
	lineNum:     "#57606a",
	hunkHdr:     "#0969da",
	fileHdr:     "#1f2328",
	gutter:      "#d0d7de",
	addInd:      "#1a7f37",
	delInd:      "#cf222e",
	ctxDim:      "#656d76",
	truncate:    "#57606a",
	dir:         "#0969da",
	file:        "#1f2328",
	cursorFg:    "#1f2328",
	cursorBg:    "#ddf4ff",
	staged:      "#1a7f37",
	unstaged:    "#9a6700",
	untracked:   "#57606a",
	border:      "#d0d7de",
	search:      "#0969da",
	title:       "#1f2328",
	movedHdr:    "#0969da",
	renamedHdr:  "#9a6700",
	summaryFg:   "#656d76",
	chromaStyle: "tokyonight-day",
}

var pal palette

var (
	lineNumSty   lipgloss.Style
	hunkHdrSty   lipgloss.Style
	fileHdrSty   lipgloss.Style
	gutterSty    lipgloss.Style
	addIndSty    lipgloss.Style
	delIndSty    lipgloss.Style
	ctxDimSty    lipgloss.Style
	dirSty       lipgloss.Style
	fileSty      lipgloss.Style
	cursorSty    lipgloss.Style
	stagedBadge  lipgloss.Style
	unstBadge    lipgloss.Style
	untrkBadge   lipgloss.Style
	borderSty    lipgloss.Style
	searchSty    lipgloss.Style
	titleSty     lipgloss.Style
	movedHdrSty  lipgloss.Style
	renamedHdrSty lipgloss.Style
	summarySty   lipgloss.Style
)

var bgColors map[diffBg]string

var darkMode bool

func initTheme(pager bool) {
	if pager {
		darkMode = systemHasDarkAppearance()
	} else {
		darkMode = termenv.HasDarkBackground()
	}
	applyTheme()
}

func systemHasDarkAppearance() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Run()
	return err == nil
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

	movedHdrSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.movedHdr))
	renamedHdrSty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.renamedHdr))
	summarySty = lipgloss.NewStyle().Foreground(lipgloss.Color(pal.summaryFg))

	bgColors = map[diffBg]string{
		bgNone: "",
		bgAdd:  pal.bgAdd,
		bgDel:  pal.bgDel,
		bgMov:  pal.bgMoved,
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
