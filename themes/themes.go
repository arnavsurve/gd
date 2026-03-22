package themes

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

func init() {
	for name, style := range registry {
		styles.Register(chroma.MustNewStyle(name, style))
	}
}

var registry = map[string]chroma.StyleEntries{}

func register(name string, entries chroma.StyleEntries) {
	registry[name] = entries
}
