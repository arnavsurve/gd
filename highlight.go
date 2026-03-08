package main

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

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
